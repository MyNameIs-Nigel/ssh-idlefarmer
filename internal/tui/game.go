// Package tui is the player-facing terminal interface.
package tui

import (
	"time"
	"unicode"

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

type errScreen struct {
	width, height int
}

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
	scrProgress
	scrStats
	scrHelp
)

var screenOrder = []screen{scrFarm, scrMarket, scrLand, scrRebirth, scrProgress, scrStats, scrHelp}

type overlay int

const (
	ovNone overlay = iota
	ovOnboarding
	ovAway
	ovPicker
	ovUpgrade
	ovRebirthConfirm
	ovName
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

	scr         screen
	overlay     overlay
	cursor      int
	pickerIdx   int
	marketIdx   int
	upgradeIdx  int
	progressIdx int
	nameInput   string

	notices    []notice
	away       sim.Events
	kickReason string
	quitAt     int64

	idleTimeout int64
	lastInput   int64
}

func NewGame(id identity.SessionIdentity, res game.AttachResult, c *content.Content, width, height int, now int64, idleTimeout int64) *Game {
	g := &Game{
		sess:        res.Session,
		content:     c,
		id:          id,
		snap:        game.Snapshot{State: sim.New(c, 0, now), Now: now},
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

// Init starts exactly one tick chain: refresh delivers an immediate tickMsg,
// and every tickMsg handler schedules the next tick. Adding tickCmd here too
// would run a second, parallel 1Hz chain forever.
func (g *Game) Init() tea.Cmd {
	return tea.Batch(g.refresh(), g.waitKick())
}

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
			return g, tea.Quit
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
			return g, tickCmd()
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
	case ovUpgrade:
		return g.handleUpgradeKey(key)
	case ovRebirthConfirm:
		return g.handleRebirthConfirmKey(key)
	case ovName:
		return g.handleNameKey(key, msg)
	}

	// Global keys.
	switch key {
	case "q":
		if g.scr == scrHelp {
			g.scr = scrFarm
			return g, nil
		}
		return g, tea.Quit
	case "g":
		return g.redeemGift()
	case "1":
		g.scr = scrFarm
		return g, nil
	case "2":
		g.scr = scrMarket
		return g, nil
	case "3":
		g.scr = scrLand
		return g, nil
	case "4":
		g.scr = scrRebirth
		return g, nil
	case "5":
		g.scr = scrProgress
		return g, nil
	case "6":
		g.scr = scrStats
		return g, nil
	case "tab":
		g.cycleScreen(true)
		return g, nil
	case "shift+tab":
		g.cycleScreen(false)
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
	case scrProgress:
		return g.handleProgressKey(key)
	case scrStats:
		return g.handleStatsKey(key)
	case scrHelp:
		if key == "esc" {
			g.scr = scrFarm
		}
	}
	return g, nil
}

func (g *Game) cycleScreen(forward bool) {
	for i, s := range screenOrder {
		if s == g.scr {
			if forward {
				g.scr = screenOrder[(i+1)%len(screenOrder)]
			} else {
				g.scr = screenOrder[(i-1+len(screenOrder))%len(screenOrder)]
			}
			return
		}
	}
}

func (g *Game) handleFarmKey(key string) (tea.Model, tea.Cmd) {
	st := g.snap.State
	cols := g.farmColumns()
	switch key {
	case "left":
		if g.cursor > 0 {
			g.cursor--
		}
	case "right":
		if g.cursor < len(st.Plots)-1 {
			g.cursor++
		}
	case "up":
		if g.cursor-cols >= 0 {
			g.cursor -= cols
		}
	case "down":
		if g.cursor+cols < len(st.Plots) {
			g.cursor += cols
		}
	case "u":
		g.upgradeIdx = g.cursor
		g.overlay = ovUpgrade
	case "x":
		if g.cursor < len(st.Plots) && st.Plots[g.cursor].Critter != "" {
			reward, snap, ach, err := g.sess.ShooCritter(g.now, g.cursor)
			g.applyAction(snap, ach, err)
			if err == nil {
				g.addNotice("Shooed the " + sanitizeText(st.Plots[g.cursor].Critter) + " (+" + money(reward) + " coins).")
			}
		}
	case "enter", "space", " ":
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
	crops := g.visibleCrops()
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
		if g.pickerIdx >= len(crops) {
			return g, nil
		}
		crop := crops[g.pickerIdx]
		st := g.snap.State
		snap, ach, err := g.sess.Plant(g.now, g.cursor, crop.ID)
		g.applyAction(snap, ach, err)
		if err == nil {
			g.overlay = ovNone
			if st.MercyPlantEligible(g.content, crop.ID) {
				g.addNotice("Planted " + sanitizeText(crop.Name) + " — FREE, the land provides.")
			} else {
				g.addNotice("Planted " + sanitizeText(crop.Name) + ".")
			}
		}
	}
	return g, nil
}

func (g *Game) handleUpgradeKey(key string) (tea.Model, tea.Cmd) {
	st := g.snap.State
	switch key {
	case "esc", "q":
		g.overlay = ovNone
	case "up", "k":
		if g.upgradeIdx > 0 {
			g.upgradeIdx--
		}
	case "down", "j":
		if g.upgradeIdx < len(st.Plots)-1 {
			g.upgradeIdx++
		}
	case "1":
		snap, ach, err := g.sess.UpgradePlotAuto(g.now, g.upgradeIdx, "harvest")
		g.applyAction(snap, ach, err)
		if err == nil {
			g.addNotice("Plot " + itoa(g.upgradeIdx+1) + " now auto-harvests!")
		}
	case "2":
		snap, ach, err := g.sess.UpgradePlotAuto(g.now, g.upgradeIdx, "sow")
		g.applyAction(snap, ach, err)
		if err == nil {
			g.addNotice("Plot " + itoa(g.upgradeIdx+1) + " now auto-sows!")
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
		switch it.kind {
		case "zone":
			snap, ach, err = g.sess.BuyZone(now, it.id)
		case "multiplier":
			snap, ach, err = g.sess.BuyMultiplier(now, it.id)
		case "strain":
			snap, ach, err = g.sess.BuySeedUpgrade(now, it.id)
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
	switch key {
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

func (g *Game) handleProgressKey(key string) (tea.Model, tea.Cmd) {
	st := g.snap.State
	if st.Rebirths < 1 {
		return g, nil
	}
	ups := g.content.Upgrades
	switch key {
	case "up", "k":
		if g.progressIdx > 0 {
			g.progressIdx--
		}
	case "down", "j":
		if g.progressIdx < len(ups)-1 {
			g.progressIdx++
		}
	case "enter", "space", " ", "b":
		if g.progressIdx >= len(ups) {
			return g, nil
		}
		u := ups[g.progressIdx]
		snap, ach, err := g.sess.BuyUpgrade(g.now, u.ID)
		g.applyAction(snap, ach, err)
		if err == nil {
			g.addNotice(sanitizeText(u.Name) + " is now level " + itoa(g.snap.State.UpgradeLevel(u.ID)) + ".")
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
			g.addNotice("Reborn! +" + money(gain) + " " + g.starseedLabel() + ". The land is fresh again.")
		}
	case "n", "N", "esc", "q":
		g.overlay = ovNone
	}
	return g, nil
}

func (g *Game) handleStatsKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "t":
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
	case "n":
		g.nameInput = g.snap.State.FarmName
		g.overlay = ovName
	}
	return g, nil
}

func (g *Game) handleNameKey(key string, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "q":
		g.overlay = ovNone
	case "enter":
		snap, err := g.sess.SetFarmName(g.now, g.nameInput)
		if err != nil {
			g.addNotice("Hmm: " + err.Error() + ".")
		} else {
			g.snap = snap
			g.overlay = ovNone
			g.addNotice("Your farm is now called " + sanitizeText(g.nameInput) + ".")
		}
	case "backspace":
		runes := []rune(g.nameInput)
		if len(runes) > 0 {
			g.nameInput = string(runes[:len(runes)-1])
		}
	default:
		if key == "space" {
			key = " "
		}
		// Accept any single printable rune (not just ASCII), capped at the
		// sim's 24-rune limit so the overlay can't grow unbounded from
		// held-down keys before Enter validates it.
		r := []rune(key)
		if len(r) == 1 && unicode.IsPrint(r[0]) && len([]rune(g.nameInput)) < 24 {
			g.nameInput += string(r)
		}
	}
	return g, nil
}

func (g *Game) redeemGift() (tea.Model, tea.Cmd) {
	res, snap, ach, err := g.sess.RedeemGift(g.now)
	g.applyAction(snap, ach, err)
	if err != nil {
		return g, nil
	}
	if res.Starseeds > 0 {
		g.addNotice("📦 Parcel opened! +" + money(res.Starseeds) + " " + g.starseedLabel() + "!")
	} else {
		g.addNotice("📦 Parcel opened! +" + money(res.Coins) + " coins!")
	}
	return g, nil
}

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
	if res.Golden {
		g.addNotice("✨ GOLDEN HARVEST! " + sanitizeText(name) + " (+" + money(res.Payout) + ")")
		return
	}
	if res.Failed && crop != nil {
		g.addNotice("💥 " + sanitizeText(name) + " failed — salvaged " + money(res.Payout) + " coins (" + salvageFractionLabel(g.snap.State.SeedUpgradeLevel(crop.ID)) + " of normal).")
		return
	}
	text := "Harvested " + sanitizeText(name) + " (+" + money(res.Payout) + ")"
	if res.Discovery > 0 {
		text += " and found " + money(res.Discovery) + " coins in the soil!"
	}
	g.addNotice(text)
}

func salvageFractionLabel(strainLevel int) string {
	switch strainLevel {
	case 0:
		return "1/8"
	case 1:
		return "1/4"
	case 2:
		return "1/2"
	default:
		return "3/4"
	}
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
		g.addNotice("⚙ Auto-plots gathered crops (+" + money(ev.AutoCoins) + ").")
	}
	if ev.GoldenHarvests > 0 {
		g.addNotice("✨ " + itoa(ev.GoldenHarvests) + " golden harvest(s)!")
	}
	for id, n := range ev.FailedHarvests {
		name := id
		if crop := g.content.Crop(id); crop != nil {
			name = crop.Name
		}
		g.addNotice("💥 " + itoa(n) + "× " + sanitizeText(name) + " failed while you were away.")
	}
	if ev.GiftArrived {
		g.addNotice("📦 A parcel waits at the gate — press g to open it.")
	}
	if ev.EventStarted != "" {
		if e := g.content.EventByID(ev.EventStarted); e != nil {
			g.addNotice("📰 " + sanitizeText(e.Name) + ": " + sanitizeText(e.Description))
		}
	}
	for _, c := range ev.CritterVisits {
		g.addNotice("A " + sanitizeText(c) + " visited an empty plot.")
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

func (g *Game) starseedLabel() string { return g.content.StarseedLabel() }

func (g *Game) visibleCrops() []content.Crop {
	return sim.VisibleCrops(g.snap.State, g.content)
}

type marketItem struct {
	id, name, desc string
	cost           int64
	kind           string // multiplier, strain, zone
	owned          bool
	locked         bool
	gate           content.Unlock
	level, maxLvl  int
}

func (g *Game) marketItems() []marketItem {
	st := g.snap.State
	var items []marketItem
	for i := range g.content.Multipliers {
		m := &g.content.Multipliers[i]
		lvl := st.MultiplierLevel(m.ID)
		items = append(items, marketItem{
			id: m.ID, name: m.Name, desc: m.Description,
			cost: st.MultiplierCost(m), kind: "multiplier",
			locked: false, level: lvl, maxLvl: m.MaxLevel,
		})
	}
	for i := range g.content.SeedUpgrades {
		su := &g.content.SeedUpgrades[i]
		crop := g.content.Crop(su.CropID)
		locked := crop != nil && !st.Unlocked(crop.Unlock)
		items = append(items, marketItem{
			id: su.ID, name: su.Name, desc: su.Description,
			cost: st.SeedUpgradeCost(su), kind: "strain",
			locked: locked, level: st.SeedUpgradeLevel(su.CropID), maxLvl: su.MaxLevel,
		})
	}
	for _, z := range g.content.Zones {
		items = append(items, marketItem{
			id: z.ID, name: z.Name, desc: z.Description, cost: z.Cost, kind: "zone",
			owned: st.Zones[z.ID], locked: !st.Unlocked(z.Unlock), gate: z.Unlock,
		})
	}
	return items
}

func itoa(n int) string {
	return money(int64(n))
}
