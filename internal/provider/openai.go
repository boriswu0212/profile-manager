package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type OpenAI struct {
	baseURL string
	apiKey  string
}

func NewOpenAI(baseURL, apiKey string) *OpenAI {
	return &OpenAI{baseURL: baseURL, apiKey: apiKey}
}

type openaiModelsResponse struct {
	Data []openaiModel `json:"data"`
}

type openaiModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

func (o *OpenAI) ListModels(ctx context.Context) ([]ModelInfo, error) {
	url := apiBase(o.baseURL) + "/models"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned %d", resp.StatusCode)
	}

	var result openaiModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	var models []ModelInfo
	for _, m := range result.Data {
		createdAt := ""
		if m.Created > 0 {
			createdAt = time.Unix(m.Created, 0).UTC().Format("2006-01-02")
		}
		models = append(models, ModelInfo{
			ID:          m.ID,
			DisplayName: m.OwnedBy,
			CreatedAt:   createdAt,
		})
	}

	return models, nil
}
