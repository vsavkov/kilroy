package modeldb

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type openRouterPayload struct {
	Data []openRouterModel `json:"data"`
}

type openRouterModel struct {
	ID                  string   `json:"id"`
	ContextLength       int      `json:"context_length"`
	SupportedParameters []string `json:"supported_parameters"`
	Architecture        struct {
		InputModalities  []string `json:"input_modalities"`
		OutputModalities []string `json:"output_modalities"`
	} `json:"architecture"`
	Pricing struct {
		Prompt     string `json:"prompt"`
		Completion string `json:"completion"`
	} `json:"pricing"`
	TopProvider struct {
		ContextLength       int `json:"context_length"`
		MaxCompletionTokens int `json:"max_completion_tokens"`
	} `json:"top_provider"`
}

// LoadCatalogFromOpenRouterJSON loads model metadata from OpenRouter
// /api/v1/models payload shape: {"data":[...]}.
func LoadCatalogFromOpenRouterJSON(path string) (*Catalog, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(b)
	sha := hex.EncodeToString(sum[:])

	var payload openRouterPayload
	if err := json.Unmarshal(b, &payload); err != nil {
		return nil, err
	}

	models := make(map[string]ModelEntry, len(payload.Data))
	for _, m := range payload.Data {
		id := strings.TrimSpace(m.ID)
		if id == "" {
			continue
		}
		provider := providerFromOpenRouterID(id)
		ctxWindow := m.ContextLength
		if ctxWindow == 0 {
			ctxWindow = m.TopProvider.ContextLength
		}
		var maxOut *int
		if m.TopProvider.MaxCompletionTokens > 0 {
			v := m.TopProvider.MaxCompletionTokens
			maxOut = &v
		}

		models[id] = ModelEntry{
			Provider:          provider,
			Mode:              "chat",
			ContextWindow:     ctxWindow,
			MaxOutputTokens:   maxOut,
			SupportsTools:     stringSliceContainsCI(m.SupportedParameters, "tools"),
			SupportsReasoning: stringSliceContainsCI(m.SupportedParameters, "reasoning") || stringSliceContainsCI(m.SupportedParameters, "include_reasoning"),
			SupportsVision:    stringSliceContainsCI(m.Architecture.InputModalities, "image") || stringSliceContainsCI(m.Architecture.OutputModalities, "image"),
			InputCostPerToken: parseFloatStringPtr(m.Pricing.Prompt),
			OutputCostPerToken: parseFloatStringPtr(m.Pricing.Completion),
		}
	}

	if len(models) == 0 {
		return nil, fmt.Errorf("openrouter model catalog is empty: %s", path)
	}
	return &Catalog{
		Path:   path,
		SHA256: sha,
		Models: models,
	}, nil
}

func providerFromOpenRouterID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	parts := strings.SplitN(id, "/", 2)
	if len(parts) < 2 {
		return ""
	}
	return normalizeCatalogProvider(parts[0])
}

func stringSliceContainsCI(values []string, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	for _, v := range values {
		if strings.ToLower(strings.TrimSpace(v)) == target {
			return true
		}
	}
	return false
}

func parseFloatStringPtr(v string) *float64 {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return nil
	}
	return &f
}
