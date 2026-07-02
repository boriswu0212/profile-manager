package config

import (
	"fmt"
	"testing"
)

func TestLastModelPerDirectory(t *testing.T) {
	c := &Config{}

	c.RecordLastModel("/proj/a", "tm-api", "deepseek-v4-flash")
	c.RecordLastModel("/proj/b", "tm-api", "claude-sonnet-5")
	c.RecordLastModel("/proj/a", "sub", "opus")

	if got := c.LastModel("/proj/a", "tm-api"); got != "deepseek-v4-flash" {
		t.Fatalf("dir a tm-api = %q", got)
	}
	if got := c.LastModel("/proj/b", "tm-api"); got != "claude-sonnet-5" {
		t.Fatalf("dir b tm-api = %q", got)
	}
	if got := c.LastModel("/proj/a", "sub"); got != "opus" {
		t.Fatalf("dir a sub = %q", got)
	}
	if got := c.LastModel("/proj/c", "tm-api"); got != "" {
		t.Fatalf("unknown dir should be empty, got %q", got)
	}

	// same (dir, profile) overwrites instead of duplicating
	c.RecordLastModel("/proj/a", "tm-api", "claude-4-sonnet")
	if got := c.LastModel("/proj/a", "tm-api"); got != "claude-4-sonnet" {
		t.Fatalf("after overwrite = %q", got)
	}
	if n := len(c.LastModels); n != 3 {
		t.Fatalf("expected 3 entries, got %d", n)
	}

	// empty dir or model is ignored
	c.RecordLastModel("", "tm-api", "x")
	c.RecordLastModel("/proj/a", "tm-api", "")
	if n := len(c.LastModels); n != 3 {
		t.Fatalf("empty records should be ignored, got %d entries", n)
	}

	// capped at MaxLastModels, evicting the oldest
	for i := 0; i < MaxLastModels+10; i++ {
		c.RecordLastModel(fmt.Sprintf("/bulk/%d", i), "tm-api", "m")
	}
	if n := len(c.LastModels); n != MaxLastModels {
		t.Fatalf("expected cap %d, got %d", MaxLastModels, n)
	}
	if got := c.LastModel("/proj/a", "tm-api"); got != "" {
		t.Fatalf("oldest entries should be evicted, got %q", got)
	}
}
