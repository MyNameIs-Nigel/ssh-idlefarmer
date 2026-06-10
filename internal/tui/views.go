package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mynameis-nigel/ssh-idlefarmer/internal/content"
)

// Cozy palette: soft greens and earth tones, nothing alarming.
var (
	styleTitle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("114"))
	styleHeader   = lipgloss.NewStyle().Foreground(lipgloss.Color("180"))
	styleNavOn    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("229")).Background(lipgloss.Color("22")).Padding(0, 1)
	styleNavOff   = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Padding(0, 1)
	styleHint     = lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Italic(true)
	styleNotice   = lipgloss.NewStyle().Foreground(lipgloss.Color("222"))
	styleReady    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("120"))
	styleGrowing  = lipgloss.NewStyle().Foreground(lipgloss.Color("108"))
	styleEmpty    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	styleLocked   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleSelected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("229"))
	styleValue    = lipgloss.NewStyle().Foreground(lipgloss.Color("222"))
	styleSection  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("151"))
	styleBox      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("65")).Padding(1, 2)
	stylePlotCard = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1).Width(20)
	stylePlotSel  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("114")).Padding(0, 1).Width(20)
)

func (g *Game) View() tea.View {
	if g.width < 36 || g.height < 10 {
		return tea.NewView("\n  This farm needs a bigger window\n  (at least 36×10). Resize to play,\n  or press q to leave.\n")
	}

	var body string
	switch g.overlay {
	case ovOnboarding:
		body = g.viewOnboarding()
	case ovAway:
		body = g.viewAway()
	case ovPicker:
		body = g.viewPicker()
	case ovRebirthConfirm:
		body = g.viewRebirthConfirm()
	case ovKicked:
		body = g.viewKicked()
	default:
		switch g.scr {
		case scrFarm:
			body = g.viewFarm()
		case scrMarket:
			body = g.viewMarket()
		case scrLand:
			body = g.viewLand()
		case scrRebirth:
			body = g.viewRebirth()
		case scrStats:
			body = g.viewStats()
		case scrHelp:
			body = g.viewHelp()
		}
	}

	sections := []string{g.viewHeader(), g.viewNav(), "", body}
	if n := g.viewNotices(); n != "" {
		sections = append(sections, "", n)
	}
	sections = append(sections, "", g.viewFooter())

	out := strings.Join(sections, "\n")
	return tea.NewView(lipgloss.NewStyle().MaxWidth(g.width).MaxHeight(g.height).Render(out))
}

func (g *Game) viewHeader() string {
	st := g.snap.State
	left := styleTitle.Render("🌾 ssh-idlefarmer") + styleHeader.Render("  ·  "+sanitizeText(g.id.Slot))
	right := styleValue.Render("⛀ " + money(st.Coins) + " coins")
	if st.Rebirths > 0 || st.PrestigeCurrency > 0 {
		right += styleHeader.Render("  ✦ " + money(st.PrestigeCurrency) + " prestige")
	}
	gap := g.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func (g *Game) viewNav() string {
	labels := []struct {
		s    screen
		text string
	}{
		{scrFarm, "1 Farm"}, {scrMarket, "2 Market"}, {scrLand, "3 Land"},
		{scrRebirth, "4 Rebirth"}, {scrStats, "5 Stats"}, {scrHelp, "? Help"},
	}
	parts := make([]string, 0, len(labels))
	for _, l := range labels {
		if l.s == g.scr && g.overlay == ovNone {
			parts = append(parts, styleNavOn.Render(l.text))
		} else {
			parts = append(parts, styleNavOff.Render(l.text))
		}
	}
	return strings.Join(parts, "")
}

func (g *Game) viewNotices() string {
	if len(g.notices) == 0 {
		return ""
	}
	lines := make([]string, 0, len(g.notices))
	for _, n := range g.notices {
		lines = append(lines, styleNotice.Render(truncate(n.text, g.width-2)))
	}
	return strings.Join(lines, "\n")
}

func (g *Game) viewFooter() string {
	var hints string
	switch {
	case g.overlay == ovPicker:
		hints = "↑/↓ choose · enter plant · esc cancel"
	case g.overlay == ovRebirthConfirm:
		hints = "y rebirth · n keep farming"
	case g.overlay == ovOnboarding || g.overlay == ovAway:
		hints = "press any key to continue"
	case g.scr == scrFarm:
		hints = "←↑↓→ select · enter plant/harvest · a harvest all · q quit"
	case g.scr == scrMarket:
		hints = "↑/↓ select · enter buy · q quit"
	case g.scr == scrLand:
		hints = "enter buy plot · q quit"
	case g.scr == scrRebirth:
		hints = "↑/↓ select upgrade · enter buy · R rebirth · q quit"
	case g.scr == scrStats:
		hints = "t toggle lucky finds · q quit"
	default:
		hints = "1-5 screens · q quit"
	}
	return styleHint.Render(truncate(hints, g.width-1))
}

// farmColumns picks how many plot cards fit per row.
func (g *Game) farmColumns() int {
	if g.compactFarm() {
		return 1
	}
	cols := g.width / 22
	if cols < 1 {
		cols = 1
	}
	if cols > 6 {
		cols = 6
	}
	return cols
}

// compactFarm switches to one-line plot rows on small terminals.
func (g *Game) compactFarm() bool {
	rowsNeeded := (len(g.snap.State.Plots) + g.width/22 - 1) / max(g.width/22, 1)
	return g.width < 66 || g.height < rowsNeeded*3+10
}

func (g *Game) viewFarm() string {
	st := g.snap.State
	if g.cursor >= len(st.Plots) {
		g.cursor = len(st.Plots) - 1
	}
	if g.compactFarm() {
		return g.viewFarmCompact()
	}

	cols := g.farmColumns()
	var rows []string
	for start := 0; start < len(st.Plots); start += cols {
		end := start + cols
		if end > len(st.Plots) {
			end = len(st.Plots)
		}
		cards := make([]string, 0, cols)
		for i := start; i < end; i++ {
			cards = append(cards, g.plotCard(i))
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cards...))
	}
	return strings.Join(rows, "\n")
}

func (g *Game) plotCard(i int) string {
	st := g.snap.State
	plot := st.Plots[i]
	title := "Plot " + itoa(i+1)
	var line1, line2 string
	switch {
	case plot.Crop == "":
		line1 = styleEmpty.Render("· empty ·")
		line2 = styleEmpty.Render("enter to plant")
	case st.PlotReady(g.content, i, g.now):
		line1 = g.cropName(plot.Crop)
		line2 = styleReady.Render("✓ ready!")
	default:
		crop := g.content.Crop(plot.Crop)
		line1 = g.cropName(plot.Crop)
		if crop != nil {
			pct := st.PlotProgressPct(g.content, i, g.now)
			left := plot.PlantedAt + st.GrowSeconds(g.content, crop) - g.now
			line2 = styleGrowing.Render(progressBar(pct, 8) + " " + duration(left))
		} else {
			line2 = styleLocked.Render("(unknown crop)")
		}
	}
	content := styleSelected.Render(title) + "\n" + line1 + "\n" + line2
	if i == g.cursor {
		return stylePlotSel.Render(content)
	}
	return stylePlotCard.Render(content)
}

func (g *Game) viewFarmCompact() string {
	st := g.snap.State
	maxRows := g.height - 10
	if maxRows < 3 {
		maxRows = 3
	}
	start := 0
	if g.cursor >= maxRows {
		start = g.cursor - maxRows + 1
	}
	var b strings.Builder
	for i := start; i < len(st.Plots) && i < start+maxRows; i++ {
		plot := st.Plots[i]
		marker := "  "
		if i == g.cursor {
			marker = styleSelected.Render("▸ ")
		}
		var status string
		switch {
		case plot.Crop == "":
			status = styleEmpty.Render("empty")
		case st.PlotReady(g.content, i, g.now):
			status = g.cropName(plot.Crop) + " " + styleReady.Render("✓ ready!")
		default:
			crop := g.content.Crop(plot.Crop)
			if crop != nil {
				left := plot.PlantedAt + st.GrowSeconds(g.content, crop) - g.now
				pct := st.PlotProgressPct(g.content, i, g.now)
				status = g.cropName(plot.Crop) + " " + styleGrowing.Render(progressBar(pct, 6)+" "+duration(left))
			} else {
				status = styleLocked.Render("(unknown crop)")
			}
		}
		b.WriteString(marker + itoa(i+1) + ". " + status + "\n")
	}
	if start+maxRows < len(st.Plots) {
		b.WriteString(styleHint.Render("  …" + itoa(len(st.Plots)-start-maxRows) + " more below"))
	}
	return strings.TrimRight(b.String(), "\n")
}

func (g *Game) cropName(id string) string {
	if crop := g.content.Crop(id); crop != nil {
		return styleValue.Render(sanitizeText(crop.Name))
	}
	return styleLocked.Render(sanitizeText(id))
}

func (g *Game) viewPicker() string {
	st := g.snap.State
	var b strings.Builder
	b.WriteString(styleSection.Render("Plant on plot "+itoa(g.cursor+1)) + "\n\n")
	for i, crop := range g.content.Crops {
		marker := "  "
		if i == g.pickerIdx {
			marker = styleSelected.Render("▸ ")
		}
		name := sanitizeText(crop.Name)
		grow := duration(st.GrowSeconds(g.content, &g.content.Crops[i]))
		info := name + "  " + money(crop.SeedCost) + "c · " + grow + " · sells " + money(crop.SellValue) + "c"
		if crop.Archetype == "risky" {
			info += " (risky!)"
		}
		switch {
		case !st.Unlocked(crop.Unlock):
			b.WriteString(marker + styleLocked.Render(info+"  🔒 "+g.gateText(crop.Unlock)) + "\n")
		case st.Coins < crop.SeedCost:
			b.WriteString(marker + styleLocked.Render(info+"  (can't afford)") + "\n")
		default:
			b.WriteString(marker + styleValue.Render(info) + "\n")
		}
	}
	return styleBox.Render(strings.TrimRight(b.String(), "\n"))
}

func (g *Game) gateText(u content.Unlock) string {
	switch u.Kind {
	case "earnings":
		return "earn " + money(u.Value) + " lifetime coins"
	case "prestige":
		return "rebirth " + money(u.Value) + "×"
	case "zone":
		if z := g.content.ZoneByID(u.Zone); z != nil {
			return "own the " + sanitizeText(z.Name)
		}
		return "own a zone"
	}
	return ""
}

func (g *Game) viewMarket() string {
	st := g.snap.State
	var b strings.Builder

	b.WriteString(styleSection.Render("Seeds (plant from the farm screen)") + "\n")
	for i := range g.content.Crops {
		crop := &g.content.Crops[i]
		line := "  " + sanitizeText(crop.Name) + " — " + crop.Archetype + " · " +
			money(crop.SeedCost) + "c · " + duration(st.GrowSeconds(g.content, crop)) +
			" · sells " + money(crop.SellValue) + "c"
		if !st.Unlocked(crop.Unlock) {
			b.WriteString(styleLocked.Render(line+"  🔒 "+g.gateText(crop.Unlock)) + "\n")
		} else {
			b.WriteString(styleValue.Render(line) + "\n")
		}
	}

	b.WriteString("\n" + styleSection.Render("Tools & Zones") + "\n")
	items := g.marketItems()
	if g.marketIdx >= len(items) && len(items) > 0 {
		g.marketIdx = len(items) - 1
	}
	for i, it := range items {
		marker := "  "
		if i == g.marketIdx {
			marker = styleSelected.Render("▸ ")
		}
		line := sanitizeText(it.name) + " — " + money(it.cost) + "c · " + sanitizeText(it.desc)
		switch {
		case it.owned:
			b.WriteString(marker + styleReady.Render("✓ "+line) + "\n")
		case it.locked:
			b.WriteString(marker + styleLocked.Render(line+"  🔒 "+g.gateText(it.gate)) + "\n")
		case st.Coins < it.cost:
			b.WriteString(marker + styleLocked.Render(line+"  (can't afford)") + "\n")
		default:
			b.WriteString(marker + styleValue.Render(line) + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (g *Game) viewLand() string {
	st := g.snap.State
	var b strings.Builder
	b.WriteString(styleSection.Render("Your land") + "\n\n")
	b.WriteString("  Plots owned: " + styleValue.Render(itoa(len(st.Plots))) + "\n")
	cost := st.NextPlotCost(g.content)
	if cost < 0 {
		b.WriteString("  " + styleHint.Render("The farm is as big as it can get — zones add more room.") + "\n")
	} else {
		b.WriteString("  Next plot: " + styleValue.Render(money(cost)+" coins") + "\n")
		if st.Coins >= cost {
			b.WriteString("\n  " + styleReady.Render("Press enter to till new ground.") + "\n")
		} else {
			b.WriteString("\n  " + styleHint.Render("Keep harvesting — "+money(cost-st.Coins)+" more coins to afford it.") + "\n")
		}
	}
	b.WriteString("\n  " + styleHint.Render("Each plot costs more than the last; zones on the market add bigger jumps."))
	return b.String()
}

func (g *Game) viewRebirth() string {
	st := g.snap.State
	gain := st.PrestigeGain(g.content)
	var b strings.Builder
	b.WriteString(styleSection.Render("Rebirth") + "\n\n")
	b.WriteString("  This run has earned " + styleValue.Render(money(st.RunEarnings)+" coins") + ".\n")
	if st.CanRebirth(g.content) {
		b.WriteString("  Rebirthing now grants " + styleReady.Render("✦ "+money(gain)+" prestige") + ".\n")
	} else {
		b.WriteString("  " + styleHint.Render("Earn "+money(g.content.Prestige.MinEarnings)+" coins in one run to unlock rebirth.") + "\n")
	}
	b.WriteString("\n" + styleSection.Render("  Kept: ") + styleValue.Render("prestige, upgrades, achievements, crop unlocks") + "\n")
	b.WriteString(styleSection.Render("  Lost: ") + styleValue.Render("coins, plots, planted crops, tools, zones") + "\n")

	b.WriteString("\n" + styleSection.Render("Permanent upgrades (✦ "+money(st.PrestigeCurrency)+" available)") + "\n")
	for i, u := range g.content.Upgrades {
		marker := "  "
		if i == g.upgradeIdx {
			marker = styleSelected.Render("▸ ")
		}
		level := st.UpgradeLevel(u.ID)
		cost := st.UpgradeCost(&g.content.Upgrades[i])
		line := sanitizeText(u.Name) + " (Lv " + itoa(level) + "/" + itoa(u.MaxLevel) + ") — " + sanitizeText(u.Description)
		switch {
		case cost < 0:
			b.WriteString(marker + styleReady.Render("✓ "+line+"  maxed") + "\n")
		case st.PrestigeCurrency < cost:
			b.WriteString(marker + styleLocked.Render(line+"  ✦ "+money(cost)) + "\n")
		default:
			b.WriteString(marker + styleValue.Render(line+"  ✦ "+money(cost)) + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (g *Game) viewRebirthConfirm() string {
	st := g.snap.State
	gain := st.PrestigeGain(g.content)
	text := styleSection.Render("Rebirth?") + "\n\n" +
		"You will gain  " + styleReady.Render("✦ "+money(gain)+" prestige") + "\n" +
		"You will lose  " + styleValue.Render(money(st.Coins)+" coins, "+itoa(len(st.Plots))+" plots, and this run's tools") + "\n\n" +
		"Your upgrades, achievements, and crop unlocks stay forever.\n\n" +
		styleReady.Render("y") + " — yes, begin anew    " + styleHint.Render("n — keep farming")
	return styleBox.Render(text)
}

func (g *Game) viewStats() string {
	st := g.snap.State
	var b strings.Builder
	b.WriteString(styleSection.Render("This farm") + "\n")
	b.WriteString("  Save slot: " + styleValue.Render(sanitizeText(g.id.Slot)) + "\n")
	b.WriteString("  Key: " + styleHint.Render(shortFingerprint(g.id.Fingerprint)) + "\n")
	b.WriteString("  Lucky finds: " + styleValue.Render(onOff(st.FlavorEnabled)) + styleHint.Render("  (t to toggle)") + "\n")
	b.WriteString("\n" + styleSection.Render("Lifetime") + "\n")
	b.WriteString("  Earnings: " + styleValue.Render(money(st.LifetimeEarnings)+" coins") + "\n")
	b.WriteString("  Harvests: " + styleValue.Render(money(st.LifetimeHarvests)) + "\n")
	b.WriteString("  Rebirths: " + styleValue.Render(money(st.Rebirths)) + "  ·  Prestige: " + styleValue.Render("✦ "+money(st.PrestigeCurrency)) + "\n")

	b.WriteString("\n" + styleSection.Render("Achievements") + "\n")
	for _, a := range g.content.Achievements {
		if _, ok := st.Achievements[a.ID]; ok {
			b.WriteString("  " + styleReady.Render("✓ "+sanitizeText(a.Name)) + styleHint.Render(" — "+sanitizeText(a.Description)) + "\n")
		} else {
			b.WriteString("  " + styleLocked.Render("· "+sanitizeText(a.Name)+" — "+sanitizeText(a.Description)) + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (g *Game) viewHelp() string {
	return styleSection.Render("How it works") + "\n\n" +
		styleValue.Render("  Plant crops, go live your life, come back and harvest. Crops keep\n"+
			"  growing while you're away — there's nothing to lose and no rush.\n"+
			"  Earn coins, buy plots and tools, and one day rebirth for permanent\n"+
			"  bonuses that make every later run faster.") + "\n\n" +
		styleSection.Render("Keys") + "\n" +
		styleValue.Render("  1-5 / f m l r s   switch screens          ? help\n"+
			"  ←↑↓→ or hjkl      move around the farm\n"+
			"  enter / space     plant (empty) or harvest (ready)\n"+
			"  a                 harvest everything that's ready\n"+
			"  t                 toggle lucky finds (stats screen)\n"+
			"  R                 rebirth (rebirth screen, with confirmation)\n"+
			"  q / ctrl+c        leave (progress is saved automatically)") + "\n\n" +
		styleHint.Render("  Your SSH key is your identity; the username picks the save slot.\n"+
			"  ssh other@host opens a second farm under the same key.")
}

func (g *Game) viewOnboarding() string {
	text := styleTitle.Render("Welcome to your farm! 🌱") + "\n\n" +
		styleValue.Render("This is a cozy idle farm you tend over SSH. Plant something,\n"+
			"disconnect whenever you like, and it will keep growing — honest.\n\n"+
			"Move with the arrow keys, press enter on an empty plot to plant\n"+
			"your first turnip, and press ? any time for help.") + "\n\n" +
		styleHint.Render("Your key is your identity — reconnect any time, no account needed.")
	return styleBox.Render(text)
}

func (g *Game) viewAway() string {
	ev := g.away
	var b strings.Builder
	b.WriteString(styleTitle.Render("Welcome back! 🌾") + "\n\n")
	b.WriteString(styleValue.Render("You were away "+duration(ev.Elapsed)+".") + "\n")
	wrote := false
	for id, n := range ev.Matured {
		name := id
		if crop := g.content.Crop(id); crop != nil {
			name = crop.Name
		}
		b.WriteString(styleReady.Render("  🌱 "+itoa(n)+"× "+sanitizeText(name)+" matured and await harvest") + "\n")
		wrote = true
	}
	if total := totalCount(ev.AutoHarvested); total > 0 {
		b.WriteString(styleReady.Render("  🤖 The auto-harvester gathered "+itoa(total)+" crops (+"+money(ev.AutoCoins)+" coins)") + "\n")
		wrote = true
	}
	if ev.Discoveries > 0 {
		b.WriteString(styleReady.Render("  ✨ "+itoa(ev.Discoveries)+" lucky finds (+"+money(ev.DiscoveryCoins)+" coins)") + "\n")
		wrote = true
	}
	if !wrote {
		b.WriteString(styleHint.Render("  The fields rested quietly. A fine time to plant something.") + "\n")
	}
	return styleBox.Render(strings.TrimRight(b.String(), "\n"))
}

func (g *Game) viewKicked() string {
	return styleBox.Render(
		styleSection.Render("Until next time 🌙") + "\n\n" +
			styleValue.Render(sanitizeText(g.kickReason)) + "\n\n" +
			styleHint.Render("Your progress is saved. Disconnecting…"))
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func totalCount(m map[string]int) int {
	t := 0
	for _, n := range m {
		t += n
	}
	return t
}
