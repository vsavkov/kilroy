package engine

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/strongdm/kilroy/internal/attractor/model"
)

type providerCLICapabilities struct {
	SupportsVerbose bool
}

func preflightProviderCLIContracts(ctx context.Context, g *model.Graph, cfg *RunConfigFile) (map[string]providerCLICapabilities, error) {
	capsByProvider := map[string]providerCLICapabilities{}
	if g == nil || cfg == nil {
		return capsByProvider, nil
	}

	usedCLIProviders := []string{}
	seen := map[string]bool{}
	for _, n := range g.Nodes {
		if n == nil || n.Shape() != "box" {
			continue
		}
		provider := normalizeProviderKey(n.Attr("llm_provider", ""))
		if provider == "" {
			continue
		}
		backend, ok := backendForProvider(cfg, provider)
		if !ok || backend != BackendCLI {
			continue
		}
		if seen[provider] {
			continue
		}
		seen[provider] = true
		usedCLIProviders = append(usedCLIProviders, provider)
	}
	sort.Strings(usedCLIProviders)

	for _, provider := range usedCLIProviders {
		exe, _ := defaultCLIInvocation(provider, "probe-model", ".")
		if strings.TrimSpace(exe) == "" {
			return nil, fmt.Errorf("provider CLI preflight: unsupported provider %q", provider)
		}
		resolved, err := exec.LookPath(exe)
		if err != nil {
			return nil, fmt.Errorf("provider CLI preflight: %s executable not found (%s): %w", provider, exe, err)
		}
		caps := providerCLICapabilities{}
		if provider == "anthropic" {
			caps.SupportsVerbose = probeCLIFlagSupport(ctx, resolved, "--verbose")
		}
		capsByProvider[provider] = caps
	}
	return capsByProvider, nil
}

func backendForProvider(cfg *RunConfigFile, provider string) (BackendKind, bool) {
	if cfg == nil {
		return "", false
	}
	for k, v := range cfg.LLM.Providers {
		if normalizeProviderKey(k) != normalizeProviderKey(provider) {
			continue
		}
		return v.Backend, true
	}
	return "", false
}

func probeCLIFlagSupport(ctx context.Context, executable string, flag string) bool {
	flag = strings.TrimSpace(flag)
	if strings.TrimSpace(executable) == "" || flag == "" {
		return false
	}

	timeout := 3 * time.Second
	if dl, ok := ctx.Deadline(); ok {
		remaining := time.Until(dl)
		if remaining > 0 && remaining < timeout {
			timeout = remaining
		}
	}
	probeCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(probeCtx, executable, "--help")
	cmd.Stdin = strings.NewReader("")
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		return false
	}
	return strings.Contains(strings.ToLower(string(out)), strings.ToLower(flag))
}
