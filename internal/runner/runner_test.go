package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAdditionalSettings_Empty(t *testing.T) {
	result, err := loadAdditionalSettings("")
	if err != nil {
		t.Fatalf("empty path should not error: %v", err)
	}
	if result != nil {
		t.Fatalf("empty path should return nil map, got %v", result)
	}
}

func TestLoadAdditionalSettings_NotFound(t *testing.T) {
	result, err := loadAdditionalSettings("/tmp/nonexistent-pm-test-settings.json")
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if result != nil {
		t.Fatalf("missing file should return nil map, got %v", result)
	}
}

func TestLoadAdditionalSettings_ValidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	data := map[string]any{
		"key1": "value1",
		"key2": 42,
	}
	raw, _ := json.Marshal(data)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}

	result, err := loadAdditionalSettings(path)
	if err != nil {
		t.Fatalf("valid JSON should not error: %v", err)
	}
	if result["key1"] != "value1" {
		t.Fatalf("key1 = %v, want value1", result["key1"])
	}
	if result["key2"] != float64(42) {
		t.Fatalf("key2 = %v (type %T), want 42", result["key2"], result["key2"])
	}
}

func TestLoadAdditionalSettings_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := loadAdditionalSettings(path)
	if err == nil {
		t.Fatal("invalid JSON should error")
	}
}

func TestLoadAdditionalSettings_TildeExpansion(t *testing.T) {
	home, _ := os.UserHomeDir()
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	data := map[string]any{"from": "tilde"}
	raw, _ := json.Marshal(data)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}

	rel, err := filepath.Rel(home, path)
	if err != nil || filepath.IsAbs(rel) || rel[0:2] == ".." {
		t.Skip("temp dir not under home, skipping tilde test")
	}

	result, err := loadAdditionalSettings("~/" + rel)
	if err != nil {
		t.Fatalf("tilde expansion should not error: %v", err)
	}
	if result["from"] != "tilde" {
		t.Fatalf("from = %v, want tilde", result["from"])
	}
}

func TestLoadAdditionalSettings_JSONC(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.jsonc")

	// JSONC with line comment, block comment, and trailing comma
	data := "{\n  \"key1\": \"value1\", // line comment\n  /* block comment */\n  \"key2\": 42,\n}"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}

	result, err := loadAdditionalSettings(path)
	if err != nil {
		t.Fatalf("JSONC should parse: %v", err)
	}
	if result["key1"] != "value1" {
		t.Fatalf("key1 = %v, want value1", result["key1"])
	}
	if result["key2"] != float64(42) {
		t.Fatalf("key2 = %v, want 42", result["key2"])
	}
}

func TestCleanJSONC(t *testing.T) {
	tests := []struct {
		name, input, want string
	}{
		{"no change", `{"a":1}`, `{"a":1}`},
		{"line comment", "{\n  // comment\n  \"a\": 1\n}", "{\n  \n  \"a\": 1\n}"},
		{"block comment", `{"a": /* comment */ 1}`, `{"a":  1}`},
		{"trailing comma", `{"a": 1,}`, `{"a": 1}`},
		{"trailing comma in array", `["a",]`, `["a"]`},
		{"comment in string", `{"a": "// not a comment"}`, `{"a": "// not a comment"}`},
		{"block comment in string", `{"a": "/* not */"}`, `{"a": "/* not */"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(cleanJSONC([]byte(tt.input)))
			if got != tt.want {
				t.Fatalf("cleanJSONC(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
