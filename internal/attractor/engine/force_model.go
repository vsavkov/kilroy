package engine

import "strings"

func normalizeForceModels(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := map[string]string{}
	for provider, modelID := range in {
		p := normalizeProviderKey(provider)
		m := strings.TrimSpace(modelID)
		if p == "" || m == "" {
			continue
		}
		out[p] = m
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func forceModelForProvider(forceModels map[string]string, provider string) (string, bool) {
	if len(forceModels) == 0 {
		return "", false
	}
	p := normalizeProviderKey(provider)
	if p == "" {
		return "", false
	}
	modelID, ok := forceModels[p]
	if !ok {
		return "", false
	}
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return "", false
	}
	return modelID, true
}
