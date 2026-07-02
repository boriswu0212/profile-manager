package provider

import (
	"context"
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	"github.com/aws/aws-sdk-go-v2/service/bedrock/types"
)

type Bedrock struct {
	region     string
	awsProfile string
}

func NewBedrock(region, awsProfile string) *Bedrock {
	if region == "" {
		region = "us-east-1"
	}
	return &Bedrock{region: region, awsProfile: awsProfile}
}

func (b *Bedrock) ListModels(ctx context.Context) ([]ModelInfo, error) {
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(b.region),
	}
	if b.awsProfile != "" {
		opts = append(opts, awsconfig.WithSharedConfigProfile(b.awsProfile))
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}

	client := bedrock.NewFromConfig(cfg)
	providerName := "Anthropic"
	result, err := client.ListFoundationModels(ctx, &bedrock.ListFoundationModelsInput{
		ByProvider: &providerName,
	})
	if err != nil {
		return nil, fmt.Errorf("list bedrock models: %w", err)
	}

	var models []ModelInfo
	for _, m := range result.ModelSummaries {
		if m.ModelLifecycle != nil && m.ModelLifecycle.Status == types.FoundationModelLifecycleStatusLegacy {
			continue
		}
		name := ""
		if m.ModelName != nil {
			name = *m.ModelName
		}
		models = append(models, ModelInfo{
			ID:          *m.ModelId,
			DisplayName: name,
		})
	}

	return models, nil
}
