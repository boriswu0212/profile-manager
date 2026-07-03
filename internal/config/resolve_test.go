package config

import (
	"testing"

	"github.com/zalando/go-keyring"
)

func TestResolveOAuthToken(t *testing.T) {
	keyring.MockInit()

	sub := &Profile{Name: "work", Provider: ProviderSubscription, APIKey: "keychain://work"}

	if _, err := ResolveOAuthToken(sub); err == nil {
		t.Fatal("want error for missing keychain entry")
	}

	if err := keyring.Set("pm", "work", "sk-ant-oat01-test"); err != nil {
		t.Fatal(err)
	}
	if tok, err := ResolveOAuthToken(sub); err != nil || tok != "sk-ant-oat01-test" {
		t.Fatalf("keychain token: got %q, %v", tok, err)
	}

	// A bare subscription profile uses the ambient claude.ai login: no
	// token, no error.
	bare := &Profile{Name: "bare", Provider: ProviderSubscription}
	if tok, err := ResolveOAuthToken(bare); err != nil || tok != "" {
		t.Fatalf("bare profile: got %q, %v; want empty, nil", tok, err)
	}

	// Literal tokens in api_key pass through.
	lit := &Profile{Name: "lit", Provider: ProviderSubscription, APIKey: "sk-ant-oat01-literal"}
	if tok, err := ResolveOAuthToken(lit); err != nil || tok != "sk-ant-oat01-literal" {
		t.Fatalf("literal token: got %q, %v", tok, err)
	}

	// Invariant guarding `pm _resolve-key`: ResolveAPIKey must keep
	// returning nothing for subscription profiles even when a token is set.
	if tok, err := ResolveAPIKey(sub); err != nil || tok != "" {
		t.Fatalf("ResolveAPIKey(subscription) = %q, %v; want empty, nil", tok, err)
	}
}

func TestTokenFingerprint(t *testing.T) {
	a, b := TokenFingerprint("sk-ant-oat01-aaa"), TokenFingerprint("sk-ant-oat01-bbb")
	if len(a) != 8 || len(b) != 8 {
		t.Fatalf("want 8 hex chars, got %q, %q", a, b)
	}
	if a == b {
		t.Fatal("different tokens must have different fingerprints")
	}
	if a != TokenFingerprint("sk-ant-oat01-aaa") {
		t.Fatal("fingerprint must be deterministic")
	}
}

func TestDeleteOAuthTokenIdempotent(t *testing.T) {
	keyring.MockInit()

	if err := keyring.Set("pm", "gone", "sk-ant-oat01-x"); err != nil {
		t.Fatal(err)
	}
	if err := DeleteOAuthToken("gone"); err != nil {
		t.Fatalf("first delete: %v", err)
	}
	if err := DeleteOAuthToken("gone"); err != nil {
		t.Fatalf("second delete should be a no-op: %v", err)
	}
}
