package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/mynameis-nigel/ssh-idlefarmer/internal/content"
	"github.com/mynameis-nigel/ssh-idlefarmer/internal/game"
	"github.com/mynameis-nigel/ssh-idlefarmer/internal/identity"
	applog "github.com/mynameis-nigel/ssh-idlefarmer/internal/log"
	"github.com/mynameis-nigel/ssh-idlefarmer/internal/sim"
	"github.com/mynameis-nigel/ssh-idlefarmer/internal/store"
)

type fixture struct {
	mgr     *game.Manager
	content *content.Content
	id      identity.SessionIdentity
	st      *store.Store
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "tui.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	c, err := content.Load("../sim/testdata")
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{
		mgr:     game.NewManager(st, c, applog.New("error", "text"), time.Hour, game.PolicyTakeover),
		content: c,
		id:      identity.SessionIdentity{Fingerprint: "SHA256:tuitest", Slot: "farm"},
		st:      st,
	}
}

func (f *fixture) attach(t *testing.T, now int64) game.AttachResult {
	t.Helper()
	res, err := f.mgr.Attach(context.Background(), f.id, "k", now, nil)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func (f *fixture) newGame(t *testing.T, now int64) *Game {
	t.Helper()
	res := f.attach(t, now)
	t.Cleanup(res.Session.Detach)
	return NewGame(f.id, res, f.content, 100, 35, now, 0)
}

func key(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	default:
		r := []rune(s)[0]
		return tea.KeyPressMsg{Code: r, Text: s}
	}
}

func press(t *testing.T, g *Game, keys ...string) *Game {
	t.Helper()
	for _, k := range keys {
		m, _ := g.Update(key(k))
		g = m.(*Game)
	}
	return g
}

func tick(t *testing.T, g *Game, now int64) (*Game, tea.Cmd) {
	t.Helper()
	m, cmd := g.Update(tickMsg(time.Unix(now, 0)))
	return m.(*Game), cmd
}

func view(g *Game) string { return g.View().Content }

func TestOnboardingThenPlantAndHarvest(t *testing.T) {
	f := newFixture(t)
	base := time.Now().Unix()
	g := f.newGame(t, base)
	if _, err := g.sess.SetFlavor(base, false); err != nil { // deterministic payouts
		t.Fatal(err)
	}

	if !strings.Contains(view(g), "Welcome to your farm") {
		t.Fatal("new key must see onboarding")
	}
	g = press(t, g, "x") // any key dismisses
	g, _ = tick(t, g, base)
	if !strings.Contains(view(g), "Plot 1") {
		t.Fatalf("expected farm view, got:\n%s", view(g))
	}

	// Plant via the picker on the selected (empty) plot.
	g = press(t, g, "enter")
	if !strings.Contains(view(g), "Plant on plot 1") {
		t.Fatal("picker should open on an empty plot")
	}
	g = press(t, g, "enter") // first crop: turnip
	if g.snap.State.Plots[0].Crop != "turnip" {
		t.Fatalf("plant failed: %+v", g.snap.State.Plots[0])
	}
	if g.snap.State.Coins != 20 {
		t.Fatalf("coins = %d, want 20 after seed cost", g.snap.State.Coins)
	}

	// Harvest while immature: explained, not executed.
	g = press(t, g, "enter")
	if !strings.Contains(view(g), "Still growing") {
		t.Fatal("immature harvest should be explained in the UI")
	}

	// Tick past maturity: live progress turns ready, harvest pays out.
	g, _ = tick(t, g, base+120)
	if !strings.Contains(view(g), "ready") {
		t.Fatalf("expected ready marker, got:\n%s", view(g))
	}
	g = press(t, g, "enter")
	if g.snap.State.Coins != 29 {
		t.Fatalf("coins = %d, want 29 after harvest", g.snap.State.Coins)
	}
	if !strings.Contains(view(g), "Harvested") {
		t.Fatal("harvest should produce a notice")
	}
	if !strings.Contains(view(g), "Achievement") {
		t.Fatal("first harvest should toast the First Sprout achievement")
	}
}

func TestScreenNavigationAndHelp(t *testing.T) {
	f := newFixture(t)
	base := time.Now().Unix()
	g := f.newGame(t, base)
	g = press(t, g, "x")
	g, _ = tick(t, g, base)

	screens := map[string]string{
		"m": "Tools & Zones",
		"l": "Your land",
		"r": "Permanent upgrades",
		"s": "Achievements",
		"?": "How it works",
		"f": "Plot 1",
	}
	for k, want := range screens {
		g = press(t, g, k)
		if !strings.Contains(view(g), want) {
			t.Fatalf("key %q: expected %q on screen, got:\n%s", k, want, view(g))
		}
	}
}

func TestResizeRelayoutsAndTinyTerminalDegrades(t *testing.T) {
	f := newFixture(t)
	base := time.Now().Unix()
	g := f.newGame(t, base)
	g = press(t, g, "x")
	g, _ = tick(t, g, base)

	m, _ := g.Update(tea.WindowSizeMsg{Width: 20, Height: 6})
	g = m.(*Game)
	if !strings.Contains(view(g), "bigger window") {
		t.Fatal("tiny terminal should degrade gracefully")
	}

	m, _ = g.Update(tea.WindowSizeMsg{Width: 50, Height: 16})
	g = m.(*Game)
	if !strings.Contains(view(g), "1.") { // compact plot list
		t.Fatalf("expected compact farm at 50 cols, got:\n%s", view(g))
	}

	m, _ = g.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	g = m.(*Game)
	if !strings.Contains(view(g), "Plot 1") {
		t.Fatal("expected full farm cards after growing the window")
	}
}

func TestUnaffordableActionsAreExplained(t *testing.T) {
	f := newFixture(t)
	base := time.Now().Unix()
	g := f.newGame(t, base)
	g = press(t, g, "x")
	g, _ = tick(t, g, base)

	// 25 starting coins; the first plot costs 50.
	g = press(t, g, "l", "enter")
	if !strings.Contains(view(g), "not enough coins") {
		t.Fatalf("unaffordable plot should be explained, got:\n%s", view(g))
	}
	if len(g.snap.State.Plots) != 3 {
		t.Fatal("refused purchase must not change state")
	}
}

func TestAwaySummaryOnReturn(t *testing.T) {
	f := newFixture(t)
	base := time.Now().Unix()

	res := f.attach(t, base)
	if _, _, err := res.Session.Plant(base, 0, "turnip"); err != nil {
		t.Fatal(err)
	}
	res.Session.Detach()

	res2 := f.attach(t, base+5000)
	t.Cleanup(res2.Session.Detach)
	g := NewGame(f.id, res2, f.content, 100, 35, base+5000, 0)
	out := view(g)
	if !strings.Contains(out, "Welcome back") {
		t.Fatalf("expected away summary, got:\n%s", out)
	}
	if !strings.Contains(out, "Turnip") || !strings.Contains(out, "matured") {
		t.Fatalf("away summary should mention the matured turnip, got:\n%s", out)
	}
	g = press(t, g, "x")
	if strings.Contains(view(g), "Welcome back") {
		t.Fatal("away summary should dismiss on any key")
	}
}

func TestRebirthPreviewConfirmAndReset(t *testing.T) {
	f := newFixture(t)
	base := time.Now().Unix()

	// Seed a mature run directly in the store: 250k earned this run.
	res := f.attach(t, base)
	res.Session.Detach()
	st := sim.New(f.content, 7, base)
	st.Coins = 9_999
	st.RunEarnings = 250_000
	st.LifetimeEarnings = 250_000
	payload, err := st.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if err := f.st.PersistSave(context.Background(), f.id.Fingerprint, f.id.Slot, payload, st.Version, base); err != nil {
		t.Fatal(err)
	}

	g := f.newGame(t, base+1)
	g, _ = tick(t, g, base+1)
	g = press(t, g, "x") // dismiss the away/achievement overlay if shown
	g = press(t, g, "r")
	out := view(g)
	if !strings.Contains(out, "50 prestige") { // isqrt(250000/100)
		t.Fatalf("rebirth preview should show the gain, got:\n%s", out)
	}

	// R arms the confirmation; n backs out safely; y commits.
	g = press(t, g, "R")
	if !strings.Contains(view(g), "Rebirth?") {
		t.Fatal("R should open the confirmation overlay")
	}
	g = press(t, g, "n")
	if g.snap.State.Rebirths != 0 {
		t.Fatal("declining the confirm must not rebirth")
	}
	g = press(t, g, "R", "y")
	stAfter := g.snap.State
	if stAfter.Rebirths != 1 || stAfter.PrestigeCurrency != 50 {
		t.Fatalf("rebirth not applied: %+v", stAfter)
	}
	if stAfter.Coins != f.content.Start.Coins || len(stAfter.Plots) != f.content.Start.Plots {
		t.Fatal("run should reset to starting state")
	}
	if !strings.Contains(view(g), "Reborn") {
		t.Fatal("rebirth should be celebrated")
	}
}

func TestKickedOverlayThenQuit(t *testing.T) {
	f := newFixture(t)
	base := time.Now().Unix()
	g := f.newGame(t, base)
	g = press(t, g, "x")
	g, _ = tick(t, g, base)

	m, _ := g.Update(kickedMsg("Your farm was opened from another session, so this one is signing off."))
	g = m.(*Game)
	if !strings.Contains(view(g), "another session") {
		t.Fatalf("kick reason should render, got:\n%s", view(g))
	}
	// The gentle goodbye quits a few seconds later.
	_, cmd := tick(t, g, base+10)
	if cmd == nil {
		t.Fatal("expected quit command after the goodbye delay")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected QuitMsg, got %T", cmd())
	}
}

func TestHostileSlotNameIsEscapedInView(t *testing.T) {
	f := newFixture(t)
	base := time.Now().Unix()
	res := f.attach(t, base)
	t.Cleanup(res.Session.Detach)

	// Identity slots are sanitized at the door; simulate a hostile value
	// reaching the UI anyway (defense in depth).
	hostile := identity.SessionIdentity{Fingerprint: "SHA256:x", Slot: "evil\x1b[2Jslot"}
	g := NewGame(hostile, res, f.content, 100, 35, base, 0)
	g, _ = tick(t, g, base)
	if strings.Contains(view(g), "\x1b[2J") {
		t.Fatal("escape sequence from a slot name reached the terminal")
	}
}

func TestIdleSessionGetsTuckedIn(t *testing.T) {
	f := newFixture(t)
	base := time.Now().Unix()
	res := f.attach(t, base)
	t.Cleanup(res.Session.Detach)
	g := NewGame(f.id, res, f.content, 100, 35, base, 60)
	g = press(t, g, "x")

	// Activity keeps the session alive.
	g, _ = tick(t, g, base+59)
	g = press(t, g, "s") // counts as input at base+59
	g, cmd := tick(t, g, base+100)
	if cmd == nil {
		t.Fatal("tick chain must continue")
	}
	if strings.Contains(view(g), "tucked itself in") {
		t.Fatal("active session must not idle out")
	}

	// 60 idle seconds later: gentle goodbye, then quit.
	g, _ = tick(t, g, base+59+60)
	if !strings.Contains(view(g), "tucked itself in") {
		t.Fatalf("idle session should be told goodbye, got:\n%s", view(g))
	}
	_, cmd = tick(t, g, base+59+60+5)
	if cmd == nil {
		t.Fatal("expected quit after the goodbye delay")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected QuitMsg, got %T", cmd())
	}
}
