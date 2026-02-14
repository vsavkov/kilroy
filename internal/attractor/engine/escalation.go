package engine

import (
	"strings"

	"github.com/danshapiro/kilroy/internal/attractor/model"
)

const defaultRetriesBeforeEscalation = 2

// parseEscalationModels parses a comma-separated list of "provider:model" pairs
// from the escalation_models node attribute. Invalid entries (missing colon) are skipped.
func parseEscalationModels(raw string) []providerModel {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	var chain []providerModel
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		idx := strings.Index(part, ":")
		if idx < 0 {
			continue // skip malformed entries
		}
		prov := strings.TrimSpace(part[:idx])
		mod := strings.TrimSpace(part[idx+1:])
		if prov == "" || mod == "" {
			continue
		}
		chain = append(chain, providerModel{Provider: normalizeProviderKey(prov), Model: mod})
	}
	return chain
}

// retriesBeforeEscalation returns the number of same-model retries allowed before
// escalating to the next model in the chain. Read from the graph attribute
// "retries_before_escalation", defaulting to 2 (meaning 3 total attempts per model).
func retriesBeforeEscalation(g *model.Graph) int {
	if g == nil {
		return defaultRetriesBeforeEscalation
	}
	v := parseInt(g.Attrs["retries_before_escalation"], defaultRetriesBeforeEscalation)
	if v < 0 {
		return 0
	}
	return v
}
