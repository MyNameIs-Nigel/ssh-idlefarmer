// Package tui is the player-facing terminal interface: a thin presentation
// and input layer over the deterministic engine. It renders snapshots and
// sends intents; every game rule lives server-side in internal/sim, behind
// the save actor. The UI never computes an outcome itself.
package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mynameis-nigel/ssh-idlefarmer/internal/content"
	"github.com/mynameis-nigel/ssh-idlefarmer/internal/game"
	"github.com/mynameis-nigel/ssh-idlefarmer/internal/identity"
	"github.com/mynameis-nigel/ssh-idlefarmer/internal/sim"
)

// Model is the Bubble Tea model type used by the Wish middleware.
type Model = tea.Model

// ProgramOption is a Bubble Tea program option.
type ProgramOption = tea.ProgramOption

// errScreen is a minimal fallback shown if a session ever reaches the UI
// without an attached save (should not happen; never crash a session over it).
type errScreen struct {
	width, height int
}

// NewErrScreen returns the fallback model.
func NewErrScreen() Model { return errScreen{} }

func (errScreen) Init() tea.Cmd { return nil }

func (e errScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		e.width, e.height = msg.Width, msg.Height
		return e, nil
	case tea.KeyPressMsg:
		return e, tea.Quit
	}
	return e, nil
}

func (e errScreen) View() tea.View {
	msg := "🌧 The farm could not be opened just now.\nPress any key to disconnect, then try again."
	v := tea.NewView(lipgloss.Place(max(e.width, 1), max(e.height, 1), lipgloss.Center, lipgloss.Center, msg))
	v.AltScreen = true
	v.WindowTitle = windowTitle
	return v
}

type screen int

const (
	scrFarm screen = iota
	scrMarket
	scrLand
	scrRebirth
	scrStats
	scrHelp
)

var screenOrder = []screen{scrFarm, scrMarket, scrLand, scrRebirth, scrStats, scrHelp}

type overlay int

const (
	ovNone overlay = iota
	ovOnboarding
	ovAway
	ovPicker
	ovRebirthConfirm
	ovKicked
)

type notice struct {
	text    string
	expires int64
}

type (
	tickMsg   time.Time
	kickedMsg string
)

// Game is the root model for one connected session.
type Game struct {
	sess    *game.Session
	content *content.Content
	id      identity.SessionIdentity

	snap   game.Snapshot
	now    int64
	width  int
	height int

	scr        screen
	overlay    overlay
	cursor     int // selected plot on the farm
	pickerIdx  int // crop picker selection
	marketIdx  int // market item selection
	upgradeIdx int // prestige upgrade selection

	notices    []notice
	away       sim.Events
	kickReason string
	quitAt     int64 // when set, quit at this unix time (gentle goodbye)

	// Idle handling lives here, not in the SSH layer: the live tick writes
	// to the connection every second, which resets any transport-level idle
	// timer. Only real key presses count as activity.
	idleTimeout int64 // seconds; 0 disables
	lastInput   int64
}

// NewGame builds the session UI from the attach result. idleTimeout (seconds,
// 0 to disable) disconnects sessions that stop pressing keys.
func NewGame(id identity.SessionIdentity, res game.AttachResult, c *content.Content, width, height int, now int64, idleTimeout int64) *Game {
	g := &Game{
		sess:        res.Session,
		content:     c,
		id:          id,
		snap:        game.Snapshot{State: sim.New(c, 0, now), Now: now}, // replaced on first tick
		now:         now,
		width:       max(width, 1),
		height:      max(height, 1),
		idleTimeout: idleTimeout,
		lastInput:   now,
	}
	switch {
	case res.Created:
		g.overlay = ovOnboarding
	case !res.Away.Empty() || res.Away.Elapsed > 60:
		g.away = res.Away
		g.overlay = ovAway
	}
	return g
}

func (g *Game) Init() tea.Cmd {
	return tea.Batch(g.refresh(), tickCmd(), g.waitKick())
}

// refresh pulls an immediate authoritative snapshot (first paint).
func (g *Game) refresh() tea.Cmd {
	return func() tea.Msg { return tickMsg(time.Now()) }
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (g *Game) waitKick() tea.Cmd {
	return func() tea.Msg {
		reason, ok := <-g.sess.Kicked()
		if !ok {
			return kickedMsg("")
		}
		return kickedMsg(reason)
	}
}

func (g *Game) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		g.width, g.height = msg.Width, msg.Height
		return g, nil

	case kickedMsg:
		if msg == "" {
			return g, tea.Quit // normal detach elsewhere; just stop
		}
		g.kickReason = string(msg)
		g.overlay = ovKicked
		g.quitAt = g.now + 3
		return g, nil

	case tickMsg:
		g.now = time.Time(msg).Unix()
		if g.quitAt != 0 && g.now >= g.quitAt {
			return g, tea.Quit
		}
		if g.idleTimeout > 0 && g.overlay != ovKicked && g.now-g.lastInput >= g.idleTimeout {
			g.kickReason = "You drifted off, so the farm tucked itself in. Reconnect whenever you like!"
			g.overlay = ovKicked
			g.quitAt = g.now + 3
			return g, tickCmd() // keep ticking so the goodbye actually quits
		}
		snap, ev, err := g.sess.Advance(g.now)
		if err != nil {
			return g, tea.Quit
		}
		g.snap = snap
		g.eventNotices(ev)
		g.pruneNotices()
		return g, tickCmd()

	case tea.KeyPressMsg:
		g.lastInput = g.now
		return g.handleKey(msg)
	}
	return g, nil
}

func (g *Game) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Ctrl+C always works, everywhere.
	if key == "ctrl+c" {
		return g, tea.Quit
	}

	switch g.overlay {
	case ovKicked:
		return g, tea.Quit
	case ovOnboarding, ovAway:
		g.overlay = ovNone
		return g, nil
	case ovPicker:
		return g.handlePickerKey(key)
	case ovRebirthConfirm:
		return g.handleRebirthConfirmKey(key)
	}

	// Global navigation.
	switch key {
	case "q":
		return g, tea.Quit
	case "f", "1":
		g.scr = scrFarm
		return g, nil
	case "m", "2":
		g.scr = scrMarket
		return g, nil
	case "l", "3":
		g.scr = scrLand
		return g, nil
	case "r", "4":
		g.scr = scrRebirth
		return g, nil
	case "s", "5":
		g.scr = scrStats
		return g, nil
	case "?", "6":
		g.scr = scrHelp
		return g, nil
	case "tab":
		for i, s := range screenOrder {
			if s == g.scr {
				g.scr = screenOrder[(i+1)%len(screenOrder)]
				break
			}
		}
		return g, nil
	}

	switch g.scr {
	case scrFarm:
		return g.handleFarmKey(key)
	case scrMarket:
		return g.handleMarketKey(key)
	case scrLand:
		return g.handleLandKey(key)
	case scrRebirth:
		return g.handleRebirthKey(key)
	case scrStats:
		return g.handleStatsKey(key)
	case scrHelp:
		if key == "esc" {
			g.scr = scrFarm
		}
	}
	return g, nil
}

func (g *Game) handleFarmKey(key string) (tea.Model, tea.Cmd) {
	st := g.snap.State
	cols := g.farmColumns()
	switch key {
	case "left", "h":
		if g.cursor > 0 {
			g.cursor--
		}
	case "right":
		if g.cursor < len(st.Plots)-1 {
			g.cursor++
		}
	case "up", "k":
		if g.cursor-cols >= 0 {
			g.cursor -= cols
		}
	case "down", "j":
		if g.cursor+cols < len(st.Plots) {
			g.cursor += cols
		}
	case "enter", "space", " ", "p":
		if g.cursor >= len(st.Plots) {
			return g, nil
		}
		plot := st.Plots[g.cursor]
		switch {
		case plot.Crop == "":
			g.pickerIdx = 0
			g.overlay = ovPicker
		case st.PlotReady(g.content, g.cursor, g.now):
			g.doHarvest(g.cursor)
		default:
			crop := g.content.Crop(plot.Crop)
			if crop != nil {
				left := plot.PlantedAt + st.GrowSeconds(g.content, crop) - g.now
				g.addNotice("Still growing — ready in " + duration(left) + ".")
			}
		}
	case "a":
		g.harvestAllReady()
	}
	return g, nil
}

func (g *Game) handlePickerKey(key string) (tea.Model, tea.Cmd) {
	crops := g.content.Crops
	switch key {
	case "esc", "q":
		g.overlay = ovNone
	case "up", "k":
		if g.pickerIdx > 0 {
			g.pickerIdx--
		}
	case "down", "j":
		if g.pickerIdx < len(crops)-1 {
			g.pickerIdx++
		}
	case "enter", "space", " ":
		crop := crops[g.pickerIdx]
		snap, ach, err := g.sess.Plant(g.now, g.cursor, crop.ID)
		g.applyAction(snap, ach, err)
		if err == nil {
			g.overlay = ovNone
			g.addNotice("Planted " + sanitizeText(crop.Name) + ".")
		}
	}
	return g, nil
}

func (g *Game) handleMarketKey(key string) (tea.Model, tea.Cmd) {
	items := g.marketItems()
	switch key {
	case "up", "k":
		if g.marketIdx > 0 {
			g.marketIdx--
		}
	case "down", "j":
		if g.marketIdx < len(items)-1 {
			g.marketIdx++
		}
	case "enter", "space", " ", "b":
		if g.marketIdx >= len(items) {
			return g, nil
		}
		it := items[g.marketIdx]
		now := g.now
		var (
			snap game.Snapshot
			ach  []string
			err  error
		)
		if it.isZone {
			snap, ach, err = g.sess.BuyZone(now, it.id)
		} else {
			snap, ach, err = g.sess.BuyTool(now, it.id)
		}
		g.applyAction(snap, ach, err)
		if err == nil {
			g.addNotice("Bought " + sanitizeText(it.name) + "!")
		}
	}
	return g, nil
}

func (g *Game) handleLandKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "enter", "space", " ", "b":
		cost, snap, ach, err := g.sess.BuyPlot(g.now)
		g.applyAction(snap, ach, err)
		if err == nil {
			g.addNotice("New plot tilled for " + money(cost) + " coins.")
		}
	}
	return g, nil
}

func (g *Game) handleRebirthKey(key string) (tea.Model, tea.Cmd) {
	ups := g.content.Upgrades
	switch key {
	case "up", "k":
		if g.upgradeIdx > 0 {
			g.upgradeIdx--
		}
	case "down", "j":
		if g.upgradeIdx < len(ups)-1 {
			g.upgradeIdx++
		}
	case "enter", "space", " ", "b":
		if g.upgradeIdx >= len(ups) {
			return g, nil
		}
		u := ups[g.upgradeIdx]
		snap, ach, err := g.sess.BuyUpgrade(g.now, u.ID)
		g.applyAction(snap, ach, err)
		if err == nil {
			g.addNotice(sanitizeText(u.Name) + " is now level " + itoa(g.snap.State.UpgradeLevel(u.ID)) + ".")
		}
	case "R":
		if g.snap.State.CanRebirth(g.content) {
			g.overlay = ovRebirthConfirm
		} else {
			need := g.content.Prestige.MinEarnings
			g.addNotice("Earn " + money(need) + " coins this run to rebirth (so far: " + money(g.snap.State.RunEarnings) + ").")
		}
	}
	return g, nil
}

func (g *Game) handleRebirthConfirmKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "y", "Y":
		gain, snap, ach, err := g.sess.Rebirth(g.now)
		g.applyAction(snap, ach, err)
		g.overlay = ovNone
		if err == nil {
			g.cursor = 0
			g.addNotice("Reborn! +" + money(gain) + " prestige. The land is fresh again.")
		}
	case "n", "N", "esc", "q":
		g.overlay = ovNone
	}
	return g, nil
}

func (g *Game) handleStatsKey(key string) (tea.Model, tea.Cmd) {
	if key == "t" {
		enabled := !g.snap.State.FlavorEnabled
		snap, err := g.sess.SetFlavor(g.now, enabled)
		if err == nil {
			g.snap = snap
			if enabled {
				g.addNotice("Lucky finds enabled.")
			} else {
				g.addNotice("Lucky finds disabled.")
			}
		}
	}
	return g, nil
}

// doHarvest sends the harvest intent for plot i.
func (g *Game) doHarvest(i int) {
	res, snap, ach, err := g.sess.Harvest(g.now, i)
	g.applyAction(snap, ach, err)
	if err != nil {
		return
	}
	crop := g.content.Crop(res.CropID)
	name := res.CropID
	if crop != nil {
		name = crop.Name
	}
	text := "Harvested " + sanitizeText(name) + " (+" + money(res.Payout) + ")"
	if res.Discovery > 0 {
		text += " and found " + money(res.Discovery) + " coins in the soil!"
	}
	g.addNotice(text)
}

func (g *Game) harvestAllReady() {
	st := g.snap.State
	total, count := int64(0), 0
	now := g.now
	for i := range st.Plots {
		if !st.PlotReady(g.content, i, g.now) {
			continue
		}
		res, snap, ach, err := g.sess.Harvest(now, i)
		if err != nil {
			continue
		}
		g.snap = snap
		g.achievementNotices(ach)
		total += res.Payout + res.Discovery
		count++
	}
	if count > 0 {
		g.addNotice("Harvested " + itoa(count) + " plots (+" + money(total) + " coins).")
	} else {
		g.addNotice("Nothing is ready yet.")
	}
}

// applyAction folds an intent result into the model: snapshot refresh,
// achievement toasts, and a friendly explanation when the engine refused.
func (g *Game) applyAction(snap game.Snapshot, ach []string, err error) {
	if err != nil {
		if err == game.ErrSessionClosed {
			return
		}
		g.addNotice("Hmm: " + err.Error() + ".")
		if snap.State != nil {
			g.snap = snap
		}
		return
	}
	g.snap = snap
	g.achievementNotices(ach)
}

func (g *Game) achievementNotices(ids []string) {
	for _, id := range ids {
		for _, a := range g.content.Achievements {
			if a.ID == id {
				g.addNotice("🏆 Achievement: " + sanitizeText(a.Name) + "!")
			}
		}
	}
}

// eventNotices surfaces live-tick events (crops maturing, auto-harvests).
func (g *Game) eventNotices(ev sim.Events) {
	for id, n := range ev.Matured {
		name := id
		if crop := g.content.Crop(id); crop != nil {
			name = crop.Name
		}
		if n == 1 {
			g.addNotice("🌱 Your " + sanitizeText(name) + " is ready to harvest!")
		} else {
			g.addNotice("🌱 " + itoa(n) + "× " + sanitizeText(name) + " are ready to harvest!")
		}
	}
	if ev.AutoCoins > 0 {
		g.addNotice("🤖 The auto-harvester gathered crops (+" + money(ev.AutoCoins) + ").")
	}
	if ev.DiscoveryCoins > 0 && len(ev.AutoHarvested) > 0 {
		g.addNotice("✨ It also found " + money(ev.DiscoveryCoins) + " spare coins.")
	}
	g.achievementNotices(ev.Achievements)
}

func (g *Game) addNotice(text string) {
	g.notices = append(g.notices, notice{text: text, expires: g.now + 6})
	if len(g.notices) > 4 {
		g.notices = g.notices[len(g.notices)-4:]
	}
}

func (g *Game) pruneNotices() {
	kept := g.notices[:0]
	for _, n := range g.notices {
		if n.expires > g.now {
			kept = append(kept, n)
		}
	}
	g.notices = kept
}

// marketItem is a buyable row on the market screen.
type marketItem struct {
	id, name, desc string
	cost           int64
	isZone         bool
	owned          bool
	locked         bool
	gate           content.Unlock
}

func (g *Game) marketItems() []marketItem {
	st := g.snap.State
	var items []marketItem
	for _, t := range g.content.Tools {
		items = append(items, marketItem{
			id: t.ID, name: t.Name, desc: t.Description, cost: t.Cost,
			owned: st.Tools[t.ID], locked: !st.Unlocked(t.Unlock), gate: t.Unlock,
		})
	}
	for _, z := range g.content.Zones {
		items = append(items, marketItem{
			id: z.ID, name: z.Name, desc: z.Description, cost: z.Cost, isZone: true,
			owned: st.Zones[z.ID], locked: !st.Unlocked(z.Unlock), gate: z.Unlock,
		})
	}
	return items
}

func itoa(n int) string {
	return money(int64(n))
}
