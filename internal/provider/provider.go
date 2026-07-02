package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/boriswu0212/profile-manager/internal/config"
)

// apiBase normalises a user-supplied base URL so that appending
// "/models" always produces the correct path regardless of whether the
// input already contains "/v1", a trailing slash, or both.
//
//	"https://api.anthropic.com"                    → "https://api.anthropic.com/v1"
//	"https://api.anthropic.com/v1"                 → "https://api.anthropic.com/v1"
//	"https://proxy.example.com/prod/aiendpoint/v1/"→ "https://proxy.example.com/prod/aiendpoint/v1"
func apiBase(raw string) string {
	u := strings.TrimRight(raw, "/")
	if strings.HasSuffix(u, "/v1") {
		return u
	}
	return u + "/v1"
}

type ModelInfo struct {
	ID              string
	DisplayName     string
	MaxInputTokens  int
	MaxOutputTokens int
	CreatedAt       string
	Capabilities    []string
}

type Provider interface {
	ListModels(ctx context.Context) ([]ModelInfo, error)
}

func ForProfile(p *config.Profile) (Provider, error) {
	key, err := config.ResolveAPIKey(p)
	if err != nil && p.Provider != config.ProviderBedrock && p.Provider != config.ProviderSubscription {
		return nil, err
	}

	switch p.Provider {
	case config.ProviderAnthropic:
		return NewAnthropic(p.BaseURL, key), nil
	case config.ProviderOpenAI:
		return NewOpenAI(p.BaseURL, key), nil
	case config.ProviderBedrock:
		return NewBedrock(p.Region, p.AWSProfile), nil
	case config.ProviderSubscription:
		return nil, fmt.Errorf("subscription mode has no API endpoint to query models")
	default:
		return nil, fmt.Errorf("unknown provider: %s", p.Provider)
	}
}
