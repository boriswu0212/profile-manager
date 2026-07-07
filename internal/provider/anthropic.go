package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type Anthropic struct {
	baseURL string
	apiKey  string
}

func NewAnthropic(baseURL, apiKey string) *Anthropic {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	return &Anthropic{baseURL: baseURL, apiKey: apiKey}
}

type anthropicModelsResponse struct {
	Data    []anthropicModel `json:"data"`
	HasMore bool             `json:"has_more"`
	LastID  string           `json:"last_id"`
}

type anthropicModel struct {
	ID             string                  `json:"id"`
	DisplayName    string                  `json:"display_name"`
	MaxInputTokens int                     `json:"max_input_tokens"`
	MaxTokens      int                     `json:"max_tokens"`
	CreatedAt      string                  `json:"created_at"`
	Capabilities   *anthropicCapabilities  `json:"capabilities,omitempty"`
	Type           string                  `json:"type"`
}

type capSupport struct {
	Supported bool `json:"supported"`
}

type anthropicCapabilities struct {
	Batch          *capSupport `json:"batch,omitempty"`
	Citations      *capSupport `json:"citations,omitempty"`
	CodeExecution  *capSupport `json:"code_execution,omitempty"`
	ImageInput     *capSupport `json:"image_input,omitempty"`
	PDFInput       *capSupport `json:"pdf_input,omitempty"`
	Thinking       *capSupport `json:"thinking,omitempty"`
}

func (c *anthropicCapabilities) list() []string {
	if c == nil {
		return nil
	}
	var caps []string
	if c.Thinking != nil && c.Thinking.Supported {
		caps = append(caps, "thinking")
	}
	if c.ImageInput != nil && c.ImageInput.Supported {
		caps = append(caps, "vision")
	}
	if c.PDFInput != nil && c.PDFInput.Supported {
		caps = append(caps, "pdf")
	}
	if c.CodeExecution != nil && c.CodeExecution.Supported {
		caps = append(caps, "code")
	}
	if c.Citations != nil && c.Citations.Supported {
		caps = append(caps, "citations")
	}
	if c.Batch != nil && c.Batch.Supported {
		caps = append(caps, "batch")
	}
	return caps
}

func (a *Anthropic) ListModels(ctx context.Context) ([]ModelInfo, error) {
	var all []ModelInfo
	afterID := ""

	for {
		reqURL := fmt.Sprintf("%s/models?limit=100", apiBase(a.baseURL))
		if afterID != "" {
			reqURL += "&after_id=" + afterID
		}

		req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("x-api-key", a.apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("request models: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			// If the Anthropic models endpoint fails and the base URL is not
			// api.anthropic.com, fall back to the OpenAI-compatible endpoint.
			// Some hosts (e.g. DeepSeek) accept Anthropic-format chat requests
			// at a path like /anthropic but only expose models via OpenAI's
			// GET /v1/models with Bearer auth.
			if !isAnthropicHost(a.baseURL) {
				return a.listModelsOpenAIFallback(ctx)
			}
			return nil, fmt.Errorf("API returned %d", resp.StatusCode)
		}

		var result anthropicModelsResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("decode response: %w", err)
		}

		for _, m := range result.Data {
			all = append(all, ModelInfo{
				ID:              m.ID,
				DisplayName:     m.DisplayName,
				MaxInputTokens:  m.MaxInputTokens,
				MaxOutputTokens: m.MaxTokens,
				CreatedAt:       m.CreatedAt,
				Capabilities:    m.Capabilities.list(),
			})
		}

		if !result.HasMore {
			break
		}
		afterID = result.LastID
	}

	return all, nil
}

// isAnthropicHost returns true when the base URL points to Anthropic's own API.
func isAnthropicHost(baseURL string) bool {
	if baseURL == "" {
		return true // default resolves to api.anthropic.com
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	return u.Hostname() == "api.anthropic.com"
}

// openAIRootBase strips any Anthropic-specific path suffix (e.g. "/anthropic")
// from the base URL and returns the root base suitable for OpenAI-compatible
// endpoints. For example "https://api.deepseek.com/anthropic" becomes
// "https://api.deepseek.com/v1".
func openAIRootBase(baseURL string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return apiBase(baseURL)
	}
	// Strip any trailing path component that is an Anthropic-specific prefix.
	// We only do this when the path ends in "/anthropic" (the common pattern
	// for hosts that serve both APIs side-by-side).
	path := strings.TrimRight(u.Path, "/")
	if strings.HasSuffix(path, "/anthropic") {
		path = strings.TrimSuffix(path, "/anthropic")
	}
	// Strip /v1 if already present so apiBase can normalise.
	path = strings.TrimSuffix(path, "/v1")
	u.Path = path
	return apiBase(u.String())
}

// listModelsOpenAIFallback queries the OpenAI-compatible GET /v1/models endpoint
// using Bearer authentication. It is used when the Anthropic endpoint is not
// available on the configured host.
func (a *Anthropic) listModelsOpenAIFallback(ctx context.Context) ([]ModelInfo, error) {
	fallbackURL := openAIRootBase(a.baseURL) + "/models"

	req, err := http.NewRequestWithContext(ctx, "GET", fallbackURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create fallback request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+a.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fallback request models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned %d", resp.StatusCode)
	}

	var result openaiModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode fallback response: %w", err)
	}

	var models []ModelInfo
	for _, m := range result.Data {
		models = append(models, ModelInfo{
			ID:          m.ID,
			DisplayName: m.OwnedBy,
		})
	}
	return models, nil
}
