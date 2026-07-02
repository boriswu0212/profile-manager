package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
		url := fmt.Sprintf("%s/models?limit=100", apiBase(a.baseURL))
		if afterID != "" {
			url += "&after_id=" + afterID
		}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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
