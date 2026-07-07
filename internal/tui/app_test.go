package tui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/boriswu0212/profile-manager/internal/config"
	"github.com/boriswu0212/profile-manager/internal/provider"
	tea "github.com/charmbracelet/bubbletea"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// Pressing "c" enters context editing mode; left/right cycles presets,
// Enter persists the selection to the config file.
func TestContextEditPresetCycle(t *testing.T) {
	path := t.TempDir() + "/config.yaml"
	cfg := &config.Config{
		Profiles: []config.Profile{
			{Name: "deepseek", Provider: config.ProviderOpenAI, Model: "deepseek-v4-pro"},
		},
	}
	m := model{
		cfg: cfg, cfgPath: path, profiles: cfg.Profiles,
		width: 90, height: 20, activePane: paneProfiles,
	}

	step := func(msg tea.KeyMsg) {
		next, _ := m.Update(msg)
		m = next.(model)
	}
	visible := func() string { return ansiRe.ReplaceAllString(m.View(), "") }

	// press c to enter context editing
	step(keyRunes("c"))
	if !m.editingContext {
		t.Fatal("expected context editing mode after pressing c")
	}
	if !strings.Contains(visible(), "Context:") {
		t.Fatal("context editing line not visible")
	}

	// default preset index should be 0 (256k)
	if m.contextPresetIdx != 0 {
		t.Fatalf("preset index = %d, want 0", m.contextPresetIdx)
	}

	// press right to cycle to 1M
	step(tea.KeyMsg{Type: tea.KeyRight})
	if m.contextPresetIdx != 1 {
		t.Fatalf("preset index = %d, want 1", m.contextPresetIdx)
	}
	if !strings.Contains(visible(), "1M") {
		t.Fatal("1M preset not shown after right arrow")
	}

	// Enter to save
	step(tea.KeyMsg{Type: tea.KeyEnter})
	if m.editingContext {
		t.Fatal("should have exited context editing after Enter")
	}
	if m.profiles[0].MaxContextTokens != 1000000 {
		t.Fatalf("MaxContextTokens = %d, want 1000000", m.profiles[0].MaxContextTokens)
	}

	// verify persisted
	saved, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Profiles[0].MaxContextTokens != 1000000 {
		t.Fatalf("saved MaxContextTokens = %d, want 1000000", saved.Profiles[0].MaxContextTokens)
	}
}

// Typing digits in context editing mode sets a custom value.
func TestContextEditCustomInput(t *testing.T) {
	path := t.TempDir() + "/config.yaml"
	cfg := &config.Config{
		Profiles: []config.Profile{
			{Name: "custom", Provider: config.ProviderOpenAI, Model: "test-model"},
		},
	}
	m := model{
		cfg: cfg, cfgPath: path, profiles: cfg.Profiles,
		width: 90, height: 20, activePane: paneProfiles,
	}

	step := func(msg tea.KeyMsg) {
		next, _ := m.Update(msg)
		m = next.(model)
	}

	step(keyRunes("c"))
	for _, r := range "500000" {
		step(keyRunes(string(r)))
	}
	if m.contextInput != "500000" {
		t.Fatalf("contextInput = %q, want 500000", m.contextInput)
	}

	step(tea.KeyMsg{Type: tea.KeyEnter})
	if m.profiles[0].MaxContextTokens != 500000 {
		t.Fatalf("MaxContextTokens = %d, want 500000", m.profiles[0].MaxContextTokens)
	}
}

// Esc cancels context editing without saving.
func TestContextEditEscCancels(t *testing.T) {
	cfg := &config.Config{
		Profiles: []config.Profile{
			{Name: "test", Provider: config.ProviderOpenAI, MaxContextTokens: 1000000},
		},
	}
	m := model{
		cfg: cfg, profiles: cfg.Profiles,
		width: 90, height: 20, activePane: paneProfiles,
	}

	step := func(msg tea.KeyMsg) {
		next, _ := m.Update(msg)
		m = next.(model)
	}

	step(keyRunes("c"))
	step(tea.KeyMsg{Type: tea.KeyRight}) // cycle to different preset
	step(tea.KeyMsg{Type: tea.KeyEsc})

	if m.editingContext {
		t.Fatal("should have exited context editing after Esc")
	}
	if m.profiles[0].MaxContextTokens != 1000000 {
		t.Fatalf("MaxContextTokens changed after Esc: %d", m.profiles[0].MaxContextTokens)
	}
}

// Profile row shows [1M] tag when MaxContextTokens is set.
func TestProfileRowShowsContextTag(t *testing.T) {
	cfg := &config.Config{
		Profiles: []config.Profile{
			{Name: "deepseek", Provider: config.ProviderOpenAI, Model: "deepseek-v4", MaxContextTokens: 1000000},
			{Name: "default", Provider: config.ProviderOpenAI, Model: "gpt-4"},
		},
	}
	m := model{cfg: cfg, profiles: cfg.Profiles, width: 90, height: 20}
	view := ansiRe.ReplaceAllString(m.View(), "")
	if !strings.Contains(view, "1M") {
		t.Fatalf("expected 1M tag in profile row for deepseek\nview:\n%s", view)
	}
}

func keyRunes(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// Model selection wraps around: up from the first item lands on the last,
// down from the last lands on the first — with the viewport following.
func TestModelCursorWrapsAround(t *testing.T) {
	cfg := &config.Config{
		Profiles: []config.Profile{
			{Name: "tm-api", Provider: config.ProviderOpenAI, Model: "claude-4-sonnet"},
		},
	}
	var models []provider.ModelInfo
	for i := 0; i < 40; i++ {
		models = append(models, provider.ModelInfo{ID: fmt.Sprintf("model-%02d", i)})
	}
	m := model{
		cfg: cfg, profiles: cfg.Profiles,
		width: 90, height: 20, activePane: paneModels,
		models: models,
	}

	visible := func() string { return ansiRe.ReplaceAllString(m.View(), "") }

	// up at the top wraps to the last item
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = next.(model)
	if m.modelCursor != len(models)-1 {
		t.Fatalf("cursor = %d, want %d", m.modelCursor, len(models)-1)
	}
	if !strings.Contains(visible(), models[len(models)-1].ID) {
		t.Fatal("last model not on screen after wrap up")
	}

	// down at the bottom wraps back to the first item
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(model)
	if m.modelCursor != 0 {
		t.Fatalf("cursor = %d, want 0", m.modelCursor)
	}
	if !strings.Contains(visible(), models[0].ID+" ") && !strings.Contains(visible(), models[0].ID+"\n") {
		t.Fatal("first model not on screen after wrap down")
	}

	// same in search mode on the filtered list
	step := func(msg tea.KeyMsg) { next, _ := m.Update(msg); m = next.(model) }
	step(keyRunes("/"))
	for _, r := range "model-3" { // matches model-30..model-39
		step(keyRunes(string(r)))
	}
	step(tea.KeyMsg{Type: tea.KeyUp})
	if got := m.visibleModels()[m.modelCursor].ID; got != "model-39" {
		t.Fatalf("filtered wrap up: cursor on %q, want model-39", got)
	}
	step(tea.KeyMsg{Type: tea.KeyDown})
	if got := m.visibleModels()[m.modelCursor].ID; got != "model-30" {
		t.Fatalf("filtered wrap down: cursor on %q, want model-30", got)
	}
}

// Search for a model, press Ctrl+S: it becomes the profile's default model
// and is persisted to the config file. Plain "s" does the same outside search.
func TestSetDefaultModelViaSearch(t *testing.T) {
	path := t.TempDir() + "/config.yaml"
	cfg := &config.Config{
		Profiles: []config.Profile{
			{Name: "tm-api", Provider: config.ProviderOpenAI, Model: "claude-4-sonnet"},
		},
	}
	m := model{
		cfg: cfg, cfgPath: path, profiles: cfg.Profiles,
		width: 90, height: 20, activePane: paneModels,
		models: []provider.ModelInfo{
			{ID: "claude-4-sonnet"}, {ID: "claude-sonnet-5"}, {ID: "deepseek-v4-pro"},
		},
	}

	step := func(msg tea.KeyMsg) {
		next, _ := m.Update(msg)
		m = next.(model)
	}

	step(keyRunes("/")) // enter search mode
	if !m.searching {
		t.Fatal("expected search mode after /")
	}
	for _, r := range "sonnet-5" {
		step(keyRunes(string(r)))
	}
	if len(m.visibleModels()) != 1 || m.visibleModels()[0].ID != "claude-sonnet-5" {
		t.Fatalf("filter mismatch: %v", m.visibleModels())
	}
	step(tea.KeyMsg{Type: tea.KeyCtrlS})

	if m.profiles[0].Model != "claude-sonnet-5" {
		t.Fatalf("profile model = %q, want claude-sonnet-5", m.profiles[0].Model)
	}
	saved, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Profiles[0].Model != "claude-sonnet-5" {
		t.Fatalf("saved model = %q, want claude-sonnet-5", saved.Profiles[0].Model)
	}

	// plain "s" in the models pane (not searching)
	step(tea.KeyMsg{Type: tea.KeyEsc}) // leave search
	step(keyRunes("j"))                // cursor → second model
	step(keyRunes("j"))                // cursor → third model
	step(keyRunes("s"))
	saved, _ = config.Load(path)
	if saved.Profiles[0].Model != "deepseek-v4-pro" {
		t.Fatalf("saved model = %q, want deepseek-v4-pro", saved.Profiles[0].Model)
	}
}

// Scrolling must keep the selected model on screen at every cursor position,
// including with the Recent section displayed (it shrinks the visible page).
func TestModelCursorStaysVisible(t *testing.T) {
	cfg := &config.Config{
		Profiles: []config.Profile{
			{Name: "tm-api", Provider: config.ProviderOpenAI, Model: "claude-4-sonnet"},
		},
	}
	var models []provider.ModelInfo
	for i := 0; i < 40; i++ {
		models = append(models, provider.ModelInfo{ID: fmt.Sprintf("model-%02d", i)})
	}
	recent := []provider.ModelInfo{{ID: "recent-a"}, {ID: "recent-b"}, {ID: "recent-c"}}

	for _, height := range []int{12, 20, 30} {
		m := model{
			cfg:          cfg,
			profiles:     cfg.Profiles,
			width:        90,
			height:       height,
			activePane:   paneModels,
			models:       models,
			recentModels: recent,
		}

		check := func(step string) {
			view := ansiRe.ReplaceAllString(m.View(), "")
			want := models[m.modelCursor].ID
			if !strings.Contains(view, want) {
				t.Fatalf("height %d, %s: selected %q (cursor %d, scroll %d) not on screen",
					height, step, want, m.modelCursor, m.modelScroll)
			}
		}

		for i := 0; i < len(models)-1; i++ {
			next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
			m = next.(model)
			check(fmt.Sprintf("down #%d", i+1))
		}
		for i := 0; i < len(models)-1; i++ {
			next, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
			m = next.(model)
			check(fmt.Sprintf("up #%d", i+1))
		}
	}
}

// A profile row must never wrap: the model abbreviation wrapping onto its own
// line reads as a separate profile entry (e.g. "sonnet" under "tm-api").
func TestProfileRowDoesNotWrap(t *testing.T) {
	cfg := &config.Config{
		DefaultProfile: "sub",
		Profiles: []config.Profile{
			{Name: "sub", Provider: config.ProviderSubscription},
			{Name: "tm-api", Provider: config.ProviderOpenAI, Model: "claude-4-sonnet"},
		},
	}

	for w := 40; w <= 120; w++ {
		m := model{cfg: cfg, profiles: cfg.Profiles, width: w, height: 20}
		for _, ln := range strings.Split(m.View(), "\n") {
			if strings.Contains(ln, "sonnet") && !strings.Contains(ln, "tm-api") {
				t.Fatalf("width %d: model label wrapped to its own line: %q", w, ln)
			}
		}
	}
}
