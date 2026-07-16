package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/b0riswu/profile-manager/internal/config"
	"github.com/b0riswu/profile-manager/internal/provider"
	"github.com/b0riswu/profile-manager/internal/runner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type pane int

const (
	paneProfiles pane = iota
	paneModels
)

type model struct {
	cfg      *config.Config
	cfgPath  string
	profiles []config.Profile
	models   []provider.ModelInfo

	activePane    pane
	profileCursor int
	modelCursor   int
	profileScroll int
	modelScroll   int

	searching      bool
	searchQuery    string
	filteredModels []provider.ModelInfo
	recentModels   []provider.ModelInfo

	loading    bool
	loadingErr string
	message    string

	editingContext   bool
	contextInput     string
	contextPresetIdx int
	contextForModel  bool
	contextModelID   string
	contextPresets   []int

	editingSettings bool
	settingsInput   string

	width  int
	height int

	shouldLaunch  bool
	launchProfile *config.Profile
	launchModel   string
}

var contextPresets = []int{0, 1000000}

func contextPresetLabel(v int) string {
	return config.FormatContextTokens(v)
}

func buildModelPresets(mod provider.ModelInfo) []int {
	if mod.MaxInputTokens > 0 && mod.MaxInputTokens != contextPresets[len(contextPresets)-1] {
		return []int{0, mod.MaxInputTokens}
	}
	return contextPresets
}

type modelsLoadedMsg struct {
	models []provider.ModelInfo
	err    error
}

func New(cfg *config.Config, cfgPath string) *tea.Program {
	m := model{
		cfg:      cfg,
		cfgPath:  cfgPath,
		profiles: cfg.Profiles,
	}
	return tea.NewProgram(m, tea.WithAltScreen())
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		m.message = ""
		m.loadingErr = ""

		if m.editingContext {
			return m.updateContextEdit(msg)
		}

		if m.editingSettings {
			return m.updateSettingsEdit(msg)
		}

		if m.searching {
			return m.updateSearch(msg)
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "/":
			if m.activePane == paneModels && len(m.models) > 0 {
				m.searching = true
				m.searchQuery = ""
				m.filteredModels = nil
				return m, nil
			}

		case "up", "k":
			if m.activePane == paneProfiles {
				if m.profileCursor > 0 {
					m.profileCursor--
					m.profileScroll = clampScroll(m.profileCursor, m.profileScroll, m.profileRowsAvail())
				}
			} else {
				m.moveModelCursor(-1)
			}

		case "down", "j":
			if m.activePane == paneProfiles {
				if m.profileCursor < len(m.profiles)-1 {
					m.profileCursor++
					m.profileScroll = clampScroll(m.profileCursor, m.profileScroll, m.profileRowsAvail())
				}
			} else {
				m.moveModelCursor(1)
			}

		case "left", "h":
			if m.activePane == paneModels {
				m.activePane = paneProfiles
			}

		case "right", "l", "m":
			if m.activePane == paneProfiles && len(m.profiles) > 0 {
				m.activePane = paneModels
				m.modelCursor = 0
				m.modelScroll = 0
				m.searching = false
				m.searchQuery = ""
				m.filteredModels = nil
				return m, m.fetchModels()
			}

		case "enter":
			if len(m.profiles) == 0 {
				return m, nil
			}
			p := m.profiles[m.profileCursor]
			m.shouldLaunch = true
			m.launchProfile = &p
			if m.activePane == paneModels {
				vis := m.visibleModels()
				if m.modelCursor < len(vis) {
					m.launchModel = vis[m.modelCursor].ID
				}
			}
			return m, tea.Quit

		case "s":
			if len(m.profiles) == 0 {
				return m, nil
			}
			if m.activePane == paneProfiles {
				m.cfg.DefaultProfile = m.profiles[m.profileCursor].Name
				_ = m.cfg.Save(m.cfgPath)
				m.message = fmt.Sprintf("Default set to %q", m.cfg.DefaultProfile)
			} else {
				vis := m.visibleModels()
				if m.modelCursor < len(vis) {
					m.setDefaultModel(vis[m.modelCursor].ID)
				}
			}

		case "d":
			if m.activePane == paneProfiles && len(m.profiles) > 0 {
				name := m.profiles[m.profileCursor].Name
				_ = m.cfg.RemoveProfile(name)
				_ = m.cfg.Save(m.cfgPath)
				m.profiles = m.cfg.Profiles
				if m.profileCursor >= len(m.profiles) && m.profileCursor > 0 {
					m.profileCursor--
				}
				m.message = fmt.Sprintf("Removed %q", name)
			}

		case "c":
			if m.activePane == paneProfiles && len(m.profiles) > 0 {
				p := m.profiles[m.profileCursor]
				m.editingContext = true
				m.contextInput = ""
				m.contextForModel = false
				m.contextPresets = contextPresets
				m.contextPresetIdx = 0
				for i, v := range m.contextPresets {
					if v == p.MaxContextTokens {
						m.contextPresetIdx = i
						break
					}
				}
			} else if m.activePane == paneModels && len(m.models) > 0 {
				vis := m.visibleModels()
				if m.modelCursor < len(vis) {
					mod := vis[m.modelCursor]
					p := m.profiles[m.profileCursor]
					m.editingContext = true
					m.contextInput = ""
					m.contextForModel = true
					m.contextModelID = mod.ID
					m.contextPresets = buildModelPresets(mod)
					m.contextPresetIdx = 0
					current := p.ResolveContextTokens(mod.ID)
					for i, v := range m.contextPresets {
						if v == current {
							m.contextPresetIdx = i
							break
						}
					}
				}
			}
		case "S":
			if m.activePane == paneProfiles && len(m.profiles) > 0 {
				m.editingSettings = true
				m.settingsInput = m.profiles[m.profileCursor].SettingsPath
			}
		}

	case modelsLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.loadingErr = msg.err.Error()
			m.models = nil
		} else {
			m.models = msg.models
			m.recentModels = m.buildRecentModels()
		}
	}

	return m, nil
}

func (m model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.searching = false
		m.searchQuery = ""
		m.filteredModels = nil
		m.modelCursor = 0
		m.modelScroll = 0
		return m, nil

	case "enter":
		vis := m.visibleModels()
		if len(vis) == 0 {
			return m, nil
		}
		p := m.profiles[m.profileCursor]
		m.shouldLaunch = true
		m.launchProfile = &p
		if m.modelCursor < len(vis) {
			m.launchModel = vis[m.modelCursor].ID
		}
		return m, tea.Quit

	case "up":
		m.moveModelCursor(-1)
		return m, nil

	case "down":
		m.moveModelCursor(1)
		return m, nil

	case "ctrl+s":
		vis := m.visibleModels()
		if m.modelCursor < len(vis) {
			m.setDefaultModel(vis[m.modelCursor].ID)
		}
		return m, nil

	case "backspace":
		if len(m.searchQuery) > 0 {
			m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
			m.applyFilter()
			m.modelCursor = 0
			m.modelScroll = 0
		}
		return m, nil

	default:
		if len(msg.String()) == 1 {
			m.searchQuery += msg.String()
			m.applyFilter()
			m.modelCursor = 0
			m.modelScroll = 0
		}
		return m, nil
	}
}

func (m model) updateContextEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	presets := m.contextPresets
	if len(presets) == 0 {
		presets = contextPresets
	}

	switch msg.String() {
	case "esc":
		m.editingContext = false
		m.contextInput = ""
		return m, nil

	case "enter":
		p := &m.profiles[m.profileCursor]
		var v int
		if m.contextInput != "" {
			v = parseContextInput(m.contextInput)
			if v <= 0 {
				m.loadingErr = "invalid context value"
				m.editingContext = false
				m.contextInput = ""
				return m, nil
			}
		} else {
			v = presets[m.contextPresetIdx]
		}

		if m.contextForModel {
			if p.ModelContext == nil {
				p.ModelContext = make(map[string]int)
			}
			if v == 0 {
				delete(p.ModelContext, m.contextModelID)
				if len(p.ModelContext) == 0 {
					p.ModelContext = nil
				}
			} else {
				p.ModelContext[m.contextModelID] = v
			}
		} else {
			p.MaxContextTokens = v
		}

		if err := m.cfg.Save(m.cfgPath); err != nil {
			m.loadingErr = fmt.Sprintf("save config: %v", err)
		} else {
			target := p.Name
			if m.contextForModel {
				target = m.contextModelID
			}
			m.message = fmt.Sprintf("Context for %q set to %s", target, config.FormatContextTokens(v))
		}
		m.editingContext = false
		m.contextInput = ""
		return m, nil

	case "left", "h":
		if m.contextInput == "" {
			m.contextPresetIdx = (m.contextPresetIdx - 1 + len(presets)) % len(presets)
		}
		return m, nil

	case "right", "l":
		if m.contextInput == "" {
			m.contextPresetIdx = (m.contextPresetIdx + 1) % len(presets)
		}
		return m, nil

	case "backspace":
		if len(m.contextInput) > 0 {
			m.contextInput = m.contextInput[:len(m.contextInput)-1]
		}
		return m, nil

	default:
		ch := msg.String()
		if len(ch) == 1 && ch[0] >= '0' && ch[0] <= '9' {
			m.contextInput += ch
		}
		return m, nil
	}
}

func (m model) updateSettingsEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.editingSettings = false
		m.settingsInput = ""
		return m, nil

	case "enter":
		p := &m.profiles[m.profileCursor]
		p.SettingsPath = strings.TrimSpace(m.settingsInput)
		if err := m.cfg.Save(m.cfgPath); err != nil {
			m.loadingErr = fmt.Sprintf("save config: %v", err)
		} else {
			m.message = fmt.Sprintf("Settings path for %q set to %q", p.Name, p.SettingsPath)
		}
		m.editingSettings = false
		m.settingsInput = ""
		return m, nil

	case "backspace":
		if len(m.settingsInput) > 0 {
			m.settingsInput = m.settingsInput[:len(m.settingsInput)-1]
		}
		return m, nil

	default:
		m.settingsInput += string(msg.Runes)
		return m, nil
	}
}

func parseContextInput(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	v := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		v = v*10 + int(c-'0')
	}
	if v <= 0 {
		return 0
	}
	return v
}

// moveModelCursor moves the model selection by delta, wrapping around at
// both ends of the (possibly filtered) list.
func (m *model) moveModelCursor(delta int) {
	vis := m.visibleModels()
	if len(vis) == 0 {
		return
	}
	m.modelCursor = ((m.modelCursor+delta)%len(vis) + len(vis)) % len(vis)
	m.modelScroll = clampScroll(m.modelCursor, m.modelScroll, m.modelRowsAvail())
}

// setDefaultModel persists id as the selected profile's default model.
// m.profiles shares its backing array with m.cfg.Profiles, so the write
// is visible to the config being saved.
func (m *model) setDefaultModel(id string) {
	if len(m.profiles) == 0 {
		return
	}
	p := &m.profiles[m.profileCursor]
	p.Model = id
	if err := m.cfg.Save(m.cfgPath); err != nil {
		m.loadingErr = fmt.Sprintf("save config: %v", err)
		return
	}
	m.message = fmt.Sprintf("Default model for %q set to %s", p.Name, id)
}

func (m model) visibleModels() []provider.ModelInfo {
	if m.searching && m.searchQuery != "" {
		return m.filteredModels
	}
	return m.models
}

func (m model) buildRecentModels() []provider.ModelInfo {
	if len(m.profiles) == 0 {
		return nil
	}
	profileName := m.profiles[m.profileCursor].Name
	recent := m.cfg.RecentForProfile(profileName)
	if len(recent) == 0 {
		return nil
	}

	modelMap := make(map[string]provider.ModelInfo)
	for _, mod := range m.models {
		modelMap[mod.ID] = mod
	}

	var out []provider.ModelInfo
	for _, r := range recent {
		if info, ok := modelMap[r.Model]; ok {
			out = append(out, info)
		}
	}
	return out
}

func (m *model) applyFilter() {
	if m.searchQuery == "" {
		m.filteredModels = nil
		return
	}
	q := strings.ToLower(m.searchQuery)
	m.filteredModels = nil
	for _, mod := range m.models {
		if strings.Contains(strings.ToLower(mod.ID), q) ||
			strings.Contains(strings.ToLower(mod.DisplayName), q) {
			m.filteredModels = append(m.filteredModels, mod)
		}
	}
}

func (m model) fetchModels() tea.Cmd {
	m.loading = true
	p := m.profiles[m.profileCursor]
	return func() tea.Msg {
		prov, err := provider.ForProfile(&p)
		if err != nil {
			return modelsLoadedMsg{err: err}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		models, err := prov.ListModels(ctx)
		return modelsLoadedMsg{models: models, err: err}
	}
}

func (m model) ShouldLaunch() bool {
	return m.shouldLaunch
}

func (m model) Launch() error {
	if m.launchProfile == nil {
		return nil
	}
	return runner.Run(m.launchProfile, m.launchModel, nil)
}

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205"))

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("229")).
			Bold(true)

	normalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	activeBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("205")).
			Padding(0, 1)

	inactiveBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	errStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	msgStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("76"))

	searchStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")).
			Bold(true)

	recentLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Bold(true)
)

func (m model) listHeight() int {
	h := m.height - 6
	if h < 3 {
		h = 3
	}
	return h
}

// maxRecentShown caps the Recent section so it cannot swallow the model list.
const maxRecentShown = 3

func (m model) visibleRecent() []provider.ModelInfo {
	if m.searching {
		return nil
	}
	r := m.recentModels
	if len(r) > maxRecentShown {
		r = r[:maxRecentShown]
	}
	return r
}

// modelRowsAvail returns how many model rows fit in one page of the models
// pane. It must mirror renderModels exactly — the same number is used to
// clamp the scroll offset, otherwise the cursor can leave the screen.
func (m model) modelRowsAvail() int {
	h := m.listHeight() - 1 // title
	if m.searching {
		h-- // search input line
	} else if n := len(m.visibleRecent()); n > 0 {
		h -= n + 2 // "Recent" label + entries + separator
	}
	if m.editingContext && m.contextForModel {
		h-- // context editing line below the selected model
	}
	h -= 2 // reserved for the ↑/↓ scroll indicators
	if h < 1 {
		h = 1
	}
	return h
}

// profileRowsAvail is the profiles-pane counterpart of modelRowsAvail.
func (m model) profileRowsAvail() int {
	h := m.listHeight() - 1 - 2 // title + ↑/↓ scroll indicators
	if m.editingContext {
		h-- // context editing line below the selected profile
	}
	if m.editingSettings {
		h-- // settings editing line below the selected profile
	}
	if h < 1 {
		h = 1
	}
	return h
}

func (m model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	lh := m.listHeight()

	// adaptive widths: small terminal → single pane, normal → split
	var leftWidth, rightWidth int
	singlePane := m.width < 60

	if singlePane {
		leftWidth = m.width - 4
		rightWidth = m.width - 4
	} else {
		leftWidth = m.width/3 - 2
		rightWidth = m.width - leftWidth - 7
	}

	// the boxes have Padding(0, 1): content narrower than the box width by 2,
	// otherwise lipgloss word-wraps overflowing rows onto extra lines
	leftContent := m.renderProfiles(leftWidth-2, lh)
	rightContent := m.renderModels(rightWidth-2, lh)

	leftBox := inactiveBoxStyle
	rightBox := inactiveBoxStyle
	if m.activePane == paneProfiles {
		leftBox = activeBoxStyle
	} else {
		rightBox = activeBoxStyle
	}

	var main string
	if singlePane {
		if m.activePane == paneProfiles {
			main = leftBox.Width(leftWidth).Height(lh).Render(leftContent)
		} else {
			main = rightBox.Width(rightWidth).Height(lh).Render(rightContent)
		}
	} else {
		left := leftBox.Width(leftWidth).Height(lh).Render(leftContent)
		right := rightBox.Width(rightWidth).Height(lh).Render(rightContent)
		main = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	}

	var footer string
	if m.editingSettings {
		footer = helpStyle.Render("Type path then [Enter] Save  [Esc] Cancel")
	} else if m.editingContext {
		footer = helpStyle.Render("[←/→] Switch preset  [0-9] Custom tokens  [Enter] Save  [Esc] Cancel")
	} else if m.searching {
		footer = helpStyle.Render("[Enter] Launch  [Ctrl+S] Set default model  [Esc] Clear search  [↑↓] Navigate")
	} else if m.activePane == paneModels {
		footer = helpStyle.Render("[Enter] Launch  [s] Set default model  [c] Context  [/] Search  [←] Profiles  [q] Quit")
	} else {
		footer = helpStyle.Render("[Enter] Launch  [m/→] Models  [s] Set default  [c] Context  [S] Settings path  [d] Delete  [q] Quit")
	}

	if m.loadingErr != "" {
		footer = errStyle.Render("Error: "+m.loadingErr) + "\n" + footer
	}
	if m.message != "" {
		footer = msgStyle.Render(m.message) + "\n" + footer
	}

	return main + "\n" + footer
}

func (m model) renderProfiles(width, height int) string {
	title := titleStyle.Render("Profiles")
	s := title + "\n"

	if len(m.profiles) == 0 {
		s += dimStyle.Render("(none)")
		return s
	}

	end := m.profileScroll + m.profileRowsAvail()
	if end > len(m.profiles) {
		end = len(m.profiles)
	}

	if m.profileScroll > 0 {
		s += dimStyle.Render(fmt.Sprintf("  ↑ %d more", m.profileScroll)) + "\n"
	}

	hasMore := end < len(m.profiles)

	for i := m.profileScroll; i < end; i++ {
		p := m.profiles[i]
		marker := "  "
		if p.Name == m.cfg.DefaultProfile {
			marker = "* "
		}

		modelShort := shortModel(p.Model)
		toolTag := ""
		if p.EffectiveTool() == config.ToolCodex {
			toolTag = "[codex] "
		}
		nameCol := p.Name
		if p.MaxContextTokens > 0 {
			nameCol += " " + config.FormatContextTokens(p.MaxContextTokens)
		}
		line := truncate(fmt.Sprintf("%-14s %s%s", nameCol, toolTag, modelShort), width-2)

		if i == m.profileCursor && m.activePane == paneProfiles {
			if m.editingContext {
				var ctxLine string
				if m.contextInput != "" {
					ctxLine = fmt.Sprintf("  Context: %s█  (enter tokens, Enter to save)", m.contextInput)
				} else {
					presets := m.contextPresets
				if len(presets) == 0 {
					presets = contextPresets
				}
				label := contextPresetLabel(presets[m.contextPresetIdx])
					ctxLine = fmt.Sprintf("  Context: ◀ %s ▶  (←/→ switch, type number, Enter to save)", label)
				}
				s += selectedStyle.Render("> "+line) + "\n"
				s += searchStyle.Render(truncate(ctxLine, width)) + "\n"
			} else if m.editingSettings {
				s += selectedStyle.Render("> "+line) + "\n"
				settingsLine := fmt.Sprintf("  Settings: %s█  (Enter to save, Esc to cancel)", m.settingsInput)
				s += searchStyle.Render(truncate(settingsLine, width)) + "\n"
			} else {
				s += selectedStyle.Render("> "+line) + "\n"
			}
		} else {
			s += normalStyle.Render(marker+line) + "\n"
		}
	}

	if hasMore {
		s += dimStyle.Render(fmt.Sprintf("  ↓ %d more", len(m.profiles)-end)) + "\n"
	}

	return s
}

func (m model) renderModels(width, height int) string {
	title := titleStyle.Render("Models")
	s := title + "\n"

	if m.loading {
		s += dimStyle.Render("Loading...")
		return s
	}

	if len(m.models) == 0 {
		s += dimStyle.Render("Press [m] or [→] to fetch")
		return s
	}

	vis := m.visibleModels()

	if m.searching {
		searchLine := searchStyle.Render("/ " + m.searchQuery + "█")
		s += searchLine + "\n"
		if len(vis) == 0 {
			s += dimStyle.Render("  (no matches)")
			return s
		}
	}

	// show recent section when not searching
	if recent := m.visibleRecent(); len(recent) > 0 {
		s += recentLabelStyle.Render("  Recent") + "\n"
		for _, mod := range recent {
			line := truncate(mod.ID, width-4)
			s += dimStyle.Render("  · "+line) + "\n"
		}
		s += dimStyle.Render("  ──────") + "\n"
	}

	end := m.modelScroll + m.modelRowsAvail()
	if end > len(vis) {
		end = len(vis)
	}

	if m.modelScroll > 0 {
		s += dimStyle.Render(fmt.Sprintf("  ↑ %d more", m.modelScroll)) + "\n"
	}

	hasMore := end < len(vis)

	var modelCtx map[string]int
	if len(m.profiles) > 0 {
		modelCtx = m.profiles[m.profileCursor].ModelContext
	}

	for i := m.modelScroll; i < end; i++ {
		mod := vis[i]
		line := mod.ID
		if mod.DisplayName != "" && mod.DisplayName != mod.ID {
			line = fmt.Sprintf("%s  %s", mod.ID, mod.DisplayName)
		}
		if v, ok := modelCtx[mod.ID]; ok && v > 0 {
			line += " " + config.FormatContextTokens(v)
		}
		line = truncate(line, width-2)

		if i == m.modelCursor && m.activePane == paneModels {
			if m.editingContext && m.contextForModel {
				presets := m.contextPresets
				if len(presets) == 0 {
					presets = contextPresets
				}
				var ctxLine string
				if m.contextInput != "" {
					ctxLine = fmt.Sprintf("  Context: %s█  (enter tokens, Enter to save)", m.contextInput)
				} else {
					label := contextPresetLabel(presets[m.contextPresetIdx])
					ctxLine = fmt.Sprintf("  Context: ◀ %s ▶  (←/→ switch, type number, Enter to save)", label)
				}
				s += selectedStyle.Render("> "+line) + "\n"
				s += searchStyle.Render(truncate(ctxLine, width)) + "\n"
			} else {
				s += selectedStyle.Render("> "+line) + "\n"
			}
		} else {
			s += normalStyle.Render("  "+line) + "\n"
		}
	}

	if hasMore {
		s += dimStyle.Render(fmt.Sprintf("  ↓ %d more", len(vis)-end)) + "\n"
	}

	return s
}

func truncate(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxWidth {
		return s
	}
	if maxWidth <= 1 {
		return "…"
	}
	return string(runes[:maxWidth-1]) + "…"
}

func clampScroll(cursor, scroll, visible int) int {
	if cursor < scroll {
		return cursor
	}
	if cursor >= scroll+visible {
		return cursor - visible + 1
	}
	return scroll
}

func shortModel(model string) string {
	if model == "" {
		return "-"
	}
	parts := map[string]string{
		"opus":   "opus",
		"sonnet": "sonnet",
		"haiku":  "haiku",
		"fable":  "fable",
	}
	for key, short := range parts {
		if contains(model, key) {
			return short
		}
	}
	if len(model) > 12 {
		return model[:12] + "…"
	}
	return model
}

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
