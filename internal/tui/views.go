package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mynameis-nigel/ssh-idlefarmer/internal/content"
	"github.com/mynameis-nigel/ssh-idlefarmer/internal/sim"
)

var (
	styleTitle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("114"))
	styleHeader   = lipgloss.NewStyle().Foreground(lipgloss.Color("180"))
	styleNavOn    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("229")).Background(lipgloss.Color("22")).Padding(0, 1)
	styleNavOff   = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Padding(0, 1)
	styleNavLock  = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Padding(0, 1)
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
	styleBanner   = lipgloss.NewStyle().Foreground(lipgloss.Color("222")).Italic(true)
	styleEvent    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("229")).Background(lipgloss.Color("54")).Padding(0, 1)
)

func (g *Game) View() tea.View {
	if g.width < minWidth || g.height < minHeight {
		return g.fullscreen("This farm needs a bigger window\n(at least 36×10). Resize to play,\nor press q to leave.")
	}
	if g.overlay != ovNone {
		return g.fullscreen(g.composeCanvas(g.overlayBox(), true))
	}
	return g.fullscreen(g.composeCanvas(g.screenBody(), false))
}

func (g *Game) overlayBox() string {
	switch g.overlay {
	case ovOnboarding:
		return g.viewOnboarding()
	case ovAway:
		return g.viewAway()
	case ovPicker:
		return g.viewPicker()
	case ovUpgrade:
		return g.viewUpgrade()
	case ovRebirthConfirm:
		return g.viewRebirthConfirm()
	case ovName:
		return g.viewName()
	case ovKicked:
		return g.viewKicked()
	}
	return ""
}

func (g *Game) screenBody() string {
	switch g.scr {
	case scrFarm:
		return g.viewFarm()
	case scrMarket:
		return g.viewMarket()
	case scrLand:
		return g.viewLand()
	case scrRebirth:
		return g.viewRebirth()
	case scrProgress:
		return g.viewProgress()
	case scrStats:
		return g.viewStats()
	case scrHelp:
		return g.viewHelp()
	}
	return ""
}

func (g *Game) viewHeader() string {
	st := g.snap.State
	title := "🌾 ssh-idlefarmer"
	if st.FarmName != "" {
		title = "🌾 " + sanitizeText(st.FarmName)
	}
	left := styleTitle.Render(title) + styleHeader.Render("  ·  "+sanitizeText(g.id.Slot))
	left += styleHint.Render("  " + moonGlyph(st, g.content) + " " + st.MoonPhaseName(g.content))
	right := styleValue.Render("⛀ " + money(st.Coins) + " coins")
	if st.Rebirths > 0 || st.PrestigeCurrency > 0 {
		right += styleHeader.Render("  ✦ " + money(st.PrestigeCurrency) + " " + g.starseedLabel())
	}
	gap := g.contentWidth() - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	header := left + strings.Repeat(" ", gap) + right

	banner := g.viewBanner()
	if banner != "" {
		header += "\n" + banner
	}
	return header
}

func moonGlyph(st *sim.State, c *content.Content) string {
	switch st.MoonPhaseName(c) {
	case "Full Moon":
		return "🌕"
	case "New Moon":
		return "🌑"
	default:
		return "🌙"
	}
}

func (g *Game) viewBanner() string {
	st := g.snap.State
	var parts []string
	if st.GiftPending {
		parts = append(parts, styleReady.Render("📦 A parcel waits at the gate — press g"))
	}
	if st.EventActive(g.now) {
		if ev := g.content.EventByID(st.EventID); ev != nil {
			left := st.EventEndsAt - g.now
			parts = append(parts, styleEvent.Render("⚡ "+sanitizeText(ev.Name)+" ("+duration(left)+")"))
		}
	}
	if len(parts) == 0 {
		return styleBanner.Render("📰 The Daily Furrow: " + g.dailyHeadline())
	}
	return strings.Join(parts, "  ")
}

func (g *Game) dailyHeadline() string {
	st := g.snap.State
	if st.EventActive(g.now) {
		if ev := g.content.EventByID(st.EventID); ev != nil {
			return strings.ToUpper(sanitizeText(ev.Name)) + " — " + strings.ToUpper(sanitizeText(ev.Description))
		}
	}
	if st.GiftPending {
		return "PARCEL DELIVERY UP ACROSS THE COUNTY"
	}
	if len(g.content.Headlines) == 0 {
		return "ALL QUIET ON THE HOMESTEAD"
	}
	idx := int(g.now/3600) % len(g.content.Headlines)
	return strings.ToUpper(sanitizeText(g.content.Headlines[idx].Text))
}

func (g *Game) viewNav() string {
	labels := []struct {
		s    screen
		text string
		lock bool
	}{
		{scrFarm, "1 Farm", false}, {scrMarket, "2 Market", false}, {scrLand, "3 Land", false},
		{scrRebirth, "4 Rebirth", false}, {scrProgress, "5 Progress", g.snap.State.Rebirths < 1},
		{scrStats, "6 Stats", false}, {scrHelp, "? Help", false},
	}
	parts := make([]string, 0, len(labels))
	for _, l := range labels {
		text := l.text
		if l.lock {
			text += " 🔒"
		}
		if l.s == g.scr && g.overlay == ovNone {
			parts = append(parts, styleNavOn.Render(text))
		} else if l.lock {
			parts = append(parts, styleNavLock.Render(text))
		} else {
			parts = append(parts, styleNavOff.Render(text))
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
		lines = append(lines, styleNotice.Render(truncate(n.text, g.contentWidth()-2)))
	}
	return strings.Join(lines, "\n")
}

func (g *Game) viewFooter() string {
	var hints string
	switch {
	case g.overlay == ovPicker:
		hints = "↑/↓ choose · enter plant · esc cancel"
	case g.overlay == ovUpgrade:
		hints = "↑/↓ plot · h auto-harvest · s auto-sow · esc cancel"
	case g.overlay == ovName:
		hints = "type name · enter save · esc cancel"
	case g.overlay == ovRebirthConfirm:
		hints = "y rebirth · n keep farming"
	case g.overlay == ovOnboarding || g.overlay == ovAway:
		hints = "press any key to continue"
	case g.scr == scrFarm:
		hints = "←↑↓→ · enter plant/harvest · a harvest all · u upgrades · g gift · q quit"
	case g.scr == scrMarket:
		hints = "↑/↓ select · enter buy · q quit"
	case g.scr == scrLand:
		hints = "enter buy plot · q quit"
	case g.scr == scrRebirth:
		hints = "R rebirth · q quit"
	case g.scr == scrProgress:
		hints = "↑/↓ select · enter buy · q quit"
	case g.scr == scrStats:
		hints = "n name farm · t toggle lucky finds · q quit"
	default:
		hints = "1-6 screens · g gift · q quit"
	}
	return styleHint.Render(truncate(hints, g.contentWidth()-1))
}

func (g *Game) farmColumns() int {
	if g.compactFarm() {
		return 1
	}
	cols := g.contentWidth() / 22
	if cols < 1 {
		cols = 1
	}
	if cols > 6 {
		cols = 6
	}
	return cols
}

func (g *Game) compactFarm() bool {
	cw, chh := g.contentWidth(), g.contentHeight()
	rowsNeeded := (len(g.snap.State.Plots) + cw/22 - 1) / max(cw/22, 1)
	return cw < 66 || chh < rowsNeeded*3+10
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

func (g *Game) plotBadges(plot sim.Plot) string {
	var b strings.Builder
	if plot.AutoHarvest {
		b.WriteString("⚙")
	}
	if plot.AutoSow {
		b.WriteString("♻")
	}
	if plot.Critter != "" && plot.Crop == "" {
		b.WriteString("🐾")
	}
	return b.String()
}

func (g *Game) plotCard(i int) string {
	st := g.snap.State
	plot := st.Plots[i]
	title := "Plot " + itoa(i+1)
	if badges := g.plotBadges(plot); badges != "" {
		title += " " + badges
	}
	var line1, line2 string
	switch {
	case plot.Crop == "":
		if plot.Critter != "" {
			line1 = styleEmpty.Render("· " + sanitizeText(plot.Critter) + " ·")
			line2 = styleHint.Render("x to shoo")
		} else {
			line1 = styleEmpty.Render("· empty ·")
			line2 = styleEmpty.Render("enter to plant")
		}
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
	maxRows := g.contentHeight() - 9
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
		badges := g.plotBadges(plot)
		var status string
		switch {
		case plot.Crop == "":
			if plot.Critter != "" {
				status = styleEmpty.Render(sanitizeText(plot.Critter))
			} else {
				status = styleEmpty.Render("empty")
			}
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
		b.WriteString(marker + itoa(i+1) + badges + ". " + status + "\n")
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
	crops := g.visibleCrops()
	var b strings.Builder
	b.WriteString(styleSection.Render("Plant on plot "+itoa(g.cursor+1)) + "\n\n")
	for i, crop := range crops {
		marker := "  "
		if i == g.pickerIdx {
			marker = styleSelected.Render("▸ ")
		}
		name := sanitizeText(crop.Name)
		grow := duration(st.GrowSeconds(g.content, &crop))
		cost := st.SeedCost(g.content, &crop)
		info := name + "  " + money(cost) + "c · " + grow + " · sells " + money(crop.SellValue) + "c"
		if crop.Archetype == "risky" {
			salvage := st.SalvageValue(g.content, &crop)
			info += " · fails to " + money(salvage) + "c (" + itoa(int(crop.FailChancePct)) + "%)"
		}
		switch {
		case !st.Unlocked(crop.Unlock):
			b.WriteString(marker + styleLocked.Render(info+"  🔒 "+g.gateText(crop.Unlock)) + "\n")
		case st.MercyPlantEligible(g.content, crop.ID):
			b.WriteString(marker + styleReady.Render(info+"  FREE — the land provides") + "\n")
		case st.Coins < cost:
			b.WriteString(marker + styleLocked.Render(info+"  (can't afford)") + "\n")
		default:
			b.WriteString(marker + styleValue.Render(info) + "\n")
		}
	}
	return styleBox.Render(strings.TrimRight(b.String(), "\n"))
}

func (g *Game) viewUpgrade() string {
	st := g.snap.State
	var b strings.Builder
	b.WriteString(styleSection.Render("Plot automation") + "\n\n")
	hCost := st.PlotAutoHarvestCost(g.content)
	sCost := g.content.PlotAutomation.AutoSowCost
	for i, plot := range st.Plots {
		marker := "  "
		if i == g.upgradeIdx {
			marker = styleSelected.Render("▸ ")
		}
		line := "Plot " + itoa(i+1)
		if plot.AutoHarvest {
			line += " ⚙"
		} else {
			line += " — harvest " + money(hCost) + "c (h)"
		}
		if plot.AutoSow {
			line += " ♻"
		} else if plot.AutoHarvest {
			line += " — sow " + money(sCost) + "c (s)"
		}
		b.WriteString(marker + styleValue.Render(line) + "\n")
	}
	b.WriteString("\n" + styleHint.Render("Auto-sow needs harvester + "+money(g.content.PlotAutomation.AutoSowMinEarnings)+" lifetime coins."))
	return styleBox.Render(strings.TrimRight(b.String(), "\n"))
}

func (g *Game) viewName() string {
	text := styleSection.Render("Name your farm") + "\n\n" +
		styleValue.Render("> "+sanitizeText(g.nameInput)+"_") + "\n\n" +
		styleHint.Render("Enter to save · Esc to cancel")
	return styleBox.Render(text)
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

	b.WriteString(styleSection.Render("Multipliers (this run)") + "\n")
	items := g.marketItems()
	if g.marketIdx >= len(items) && len(items) > 0 {
		g.marketIdx = len(items) - 1
	}
	idx := 0
	for i, it := range items {
		if it.kind != "multiplier" {
			continue
		}
		marker := "  "
		if i == g.marketIdx {
			marker = styleSelected.Render("▸ ")
		}
		line := sanitizeText(it.name) + " Lv" + itoa(it.level) + "/" + itoa(it.maxLvl) + " — " + sanitizeText(it.desc)
		switch {
		case it.level >= it.maxLvl:
			b.WriteString(marker + styleReady.Render("✓ "+line+"  maxed") + "\n")
		case st.Coins < it.cost:
			b.WriteString(marker + styleLocked.Render(line+"  "+money(it.cost)+"c") + "\n")
		default:
			b.WriteString(marker + styleValue.Render(line+"  "+money(it.cost)+"c") + "\n")
		}
		idx++
		_ = idx
	}

	b.WriteString("\n" + styleSection.Render("Hardier Strains") + "\n")
	for i, it := range items {
		if it.kind != "strain" {
			continue
		}
		marker := "  "
		if i == g.marketIdx {
			marker = styleSelected.Render("▸ ")
		}
		line := sanitizeText(it.name) + " Lv" + itoa(it.level) + "/" + itoa(it.maxLvl) + " — " + sanitizeText(it.desc)
		switch {
		case it.level >= it.maxLvl:
			b.WriteString(marker + styleReady.Render("✓ "+line+"  maxed") + "\n")
		case it.locked:
			b.WriteString(marker + styleLocked.Render(line+"  🔒") + "\n")
		case st.Coins < it.cost:
			b.WriteString(marker + styleLocked.Render(line+"  "+money(it.cost)+"c") + "\n")
		default:
			b.WriteString(marker + styleValue.Render(line+"  "+money(it.cost)+"c") + "\n")
		}
	}

	b.WriteString("\n" + styleSection.Render("Zones") + "\n")
	for i, it := range items {
		if it.kind != "zone" {
			continue
		}
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
		}
	}

	b.WriteString("\n" + styleSection.Render("Seed catalog (plant from Farm)") + "\n")
	for _, crop := range g.visibleCrops() {
		grow := duration(st.GrowSeconds(g.content, &crop))
		line := "  " + sanitizeText(crop.Name) + " — " + crop.Archetype + " · " +
			money(st.SeedCost(g.content, &crop)) + "c · " + grow +
			" · sells " + money(crop.SellValue) + "c"
		if crop.Archetype == "risky" {
			line += " · fails to " + money(st.SalvageValue(g.content, &crop)) + "c"
		}
		if !st.Unlocked(crop.Unlock) {
			b.WriteString(styleLocked.Render(line+"  🔒 "+g.gateText(crop.Unlock)) + "\n")
		} else {
			b.WriteString(styleValue.Render(line) + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (g *Game) viewRebirth() string {
	st := g.snap.State
	gain := st.PrestigeGain(g.content)
	var b strings.Builder
	b.WriteString(styleSection.Render("Rebirth") + "\n\n")
	b.WriteString("  This run has earned " + styleValue.Render(money(st.RunEarnings)+" coins") + ".\n")
	if st.CanRebirth(g.content) {
		b.WriteString("  Rebirthing now grants " + styleReady.Render("✦ "+money(gain)+" "+g.starseedLabel()) + ".\n")
	} else {
		b.WriteString("  " + styleHint.Render("Earn "+money(g.content.Prestige.MinEarnings)+" coins in one run to unlock rebirth.") + "\n")
	}
	b.WriteString("\n" + styleSection.Render("  Kept: ") + styleValue.Render(g.starseedLabel()+", upgrades, achievements, crop unlocks") + "\n")
	b.WriteString(styleSection.Render("  Lost: ") + styleValue.Render("coins, plots, crops, multipliers, zones, plot automation") + "\n")

	b.WriteString("\n" + styleSection.Render("Next rebirth unlocks") + "\n")
	nextTier := st.Rebirths + 1
	found := false
	for _, crop := range g.content.Crops {
		if crop.Unlock.Kind == "prestige" && crop.Unlock.Value == nextTier {
			b.WriteString("  " + styleValue.Render("🌱 "+sanitizeText(crop.Name)+" ("+crop.Archetype+")") + "\n")
			found = true
		}
	}
	if !found {
		b.WriteString("  " + styleHint.Render("More secrets await deeper cycles…") + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (g *Game) viewProgress() string {
	st := g.snap.State
	if st.Rebirths < 1 {
		return styleSection.Render("Progress") + "\n\n" +
			styleLocked.Render("  🔒 Rebirth once to discover what lies beyond.\n\n") +
			styleHint.Render("  The cosmos keeps its deeper rewards for those who begin anew.")
	}
	var b strings.Builder
	b.WriteString(styleSection.Render("Progress — "+g.starseedLabel()) + "\n\n")
	b.WriteString("  Balance: " + styleValue.Render("✦ "+money(st.PrestigeCurrency)) + "\n")
	b.WriteString("  Rebirths: " + styleValue.Render(money(st.Rebirths)) + "\n\n")

	b.WriteString(styleSection.Render("Lifetime upgrades") + "\n")
	for i, u := range g.content.Upgrades {
		marker := "  "
		if i == g.progressIdx {
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
		"You will gain  " + styleReady.Render("✦ "+money(gain)+" "+g.starseedLabel()) + "\n" +
		"You will lose  " + styleValue.Render(money(st.Coins)+" coins, "+itoa(len(st.Plots))+" plots, and this run's progress") + "\n\n" +
		"Your " + g.starseedLabel() + ", upgrades, and achievements stay forever.\n\n" +
		styleReady.Render("y") + " — yes, begin anew    " + styleHint.Render("n — keep farming")
	return styleBox.Render(text)
}

func (g *Game) viewStats() string {
	st := g.snap.State
	var b strings.Builder
	b.WriteString(styleSection.Render("This farm") + "\n")
	if st.FarmName != "" {
		b.WriteString("  Farm name: " + styleValue.Render(sanitizeText(st.FarmName)) + styleHint.Render("  (n to rename)") + "\n")
	} else {
		b.WriteString("  Farm name: " + styleHint.Render("(unnamed — press n)") + "\n")
	}
	b.WriteString("  Save slot: " + styleValue.Render(sanitizeText(g.id.Slot)) + "\n")
	b.WriteString("  Key: " + styleHint.Render(shortFingerprint(g.id.Fingerprint)) + "\n")
	b.WriteString("  Lucky finds: " + styleValue.Render(onOff(st.FlavorEnabled)) + styleHint.Render("  (t to toggle)") + "\n")
	b.WriteString("\n" + styleSection.Render("Lifetime") + "\n")
	b.WriteString("  Earnings: " + styleValue.Render(money(st.LifetimeEarnings)+" coins") + "\n")
	b.WriteString("  Harvests: " + styleValue.Render(money(st.LifetimeHarvests)) + "\n")
	b.WriteString("  Rebirths: " + styleValue.Render(money(st.Rebirths)) + "  ·  " + g.starseedLabel() + ": " + styleValue.Render("✦ "+money(st.PrestigeCurrency)) + "\n")

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
	ss := g.starseedLabel()
	return styleSection.Render("How it works") + "\n\n" +
		styleValue.Render("  Plant crops, go live your life, come back and harvest. Crops keep\n"+
			"  growing while you're away. Earn coins, buy plots and upgrades, and\n"+
			"  rebirth for "+ss+" — permanent bonuses that make every later run faster.") + "\n\n" +
		styleSection.Render("Keys") + "\n" +
		styleValue.Render("  1-6 / f m l r p s   switch screens          ? help\n"+
			"  ←↑↓→ or hjkl      move around the farm\n"+
			"  enter / space     plant (empty) or harvest (ready)\n"+
			"  a                 harvest everything that's ready\n"+
			"  u                 plot automation upgrades\n"+
			"  g                 redeem a gift parcel\n"+
			"  x                 shoo a critter off a plot\n"+
			"  n                 name your farm (stats screen)\n"+
			"  R                 rebirth (rebirth screen, with confirmation)\n"+
			"  q / ctrl+c        leave (progress is saved automatically)") + "\n\n" +
		styleHint.Render("  Your SSH key is your identity; the username picks the save slot.")
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
	for _, v := range ev.AwayVignettes {
		b.WriteString(styleHint.Render("  "+sanitizeText(v)) + "\n")
	}
	wrote := len(ev.AwayVignettes) > 0
	for id, n := range ev.Matured {
		name := id
		if crop := g.content.Crop(id); crop != nil {
			name = crop.Name
		}
		b.WriteString(styleReady.Render("  🌱 "+itoa(n)+"× "+sanitizeText(name)+" matured and await harvest") + "\n")
		wrote = true
	}
	if total := totalCount(ev.AutoHarvested); total > 0 {
		b.WriteString(styleReady.Render("  ⚙ Auto-plots gathered "+itoa(total)+" crops (+"+money(ev.AutoCoins)+" coins)") + "\n")
		wrote = true
	}
	if ev.GoldenHarvests > 0 {
		b.WriteString(styleReady.Render("  ✨ "+itoa(ev.GoldenHarvests)+" golden harvest(s)!") + "\n")
		wrote = true
	}
	for id, n := range ev.FailedHarvests {
		name := id
		if crop := g.content.Crop(id); crop != nil {
			name = crop.Name
		}
		b.WriteString(styleReady.Render("  💥 "+itoa(n)+"× "+sanitizeText(name)+" failed in the field") + "\n")
		wrote = true
	}
	if ev.Discoveries > 0 {
		b.WriteString(styleReady.Render("  ✨ "+itoa(ev.Discoveries)+" lucky finds (+"+money(ev.DiscoveryCoins)+" coins)") + "\n")
		wrote = true
	}
	if ev.GiftArrived {
		b.WriteString(styleReady.Render("  📦 A parcel waits at the gate — press g") + "\n")
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
