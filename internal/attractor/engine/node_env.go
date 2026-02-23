package engine

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	runIDEnvKey        = "KILROY_RUN_ID"
	nodeIDEnvKey       = "KILROY_NODE_ID"
	logsRootEnvKey     = "KILROY_LOGS_ROOT"
	stageLogsDirEnvKey = "KILROY_STAGE_LOGS_DIR"
	worktreeDirEnvKey  = "KILROY_WORKTREE_DIR"
)

// buildBaseNodeEnv constructs the base environment for any node execution.
// It:
//   - Starts from os.Environ()
//   - Strips CLAUDECODE (nested session protection)
//   - Applies resolved artifact policy env vars
//
// Both ToolHandler and CodergenRouter should use this as their starting env,
// then apply handler-specific overrides on top.
func buildBaseNodeEnv(worktreeDir string, rp ResolvedArtifactPolicy) []string {
	_ = worktreeDir
	base := os.Environ()
	base = stripEnvKey(base, "CLAUDECODE")

	normalized := normalizeResolvedArtifactPolicy(rp)
	overrides := make(map[string]string, len(normalized.Env.Vars))
	for k, v := range normalized.Env.Vars {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		overrides[key] = v
	}
	return mergeEnvWithOverrides(base, overrides)
}

// stripEnvKey removes all entries with the given key from an env slice.
func stripEnvKey(env []string, key string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env))
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) || entry == key {
			continue
		}
		out = append(out, entry)
	}
	return out
}

// buildStageRuntimeEnv returns stable per-stage environment variables that
// help codergen/tool nodes find their run-local state (logs, worktree, etc.).
func buildStageRuntimeEnv(execCtx *Execution, nodeID string) map[string]string {
	out := map[string]string{}
	if execCtx == nil {
		return out
	}
	if execCtx.Engine != nil {
		if runID := strings.TrimSpace(execCtx.Engine.Options.RunID); runID != "" {
			out[runIDEnvKey] = runID
		}
	}
	if id := strings.TrimSpace(nodeID); id != "" {
		out[nodeIDEnvKey] = id
	}
	if logsRoot := strings.TrimSpace(execCtx.LogsRoot); logsRoot != "" {
		out[logsRootEnvKey] = logsRoot
		if id := strings.TrimSpace(nodeID); id != "" {
			out[stageLogsDirEnvKey] = filepath.Join(logsRoot, id)
		}
	}
	if worktree := strings.TrimSpace(execCtx.WorktreeDir); worktree != "" {
		out[worktreeDirEnvKey] = worktree
	}
	return out
}

func buildStageRuntimePreamble(execCtx *Execution, nodeID string) string {
	if execCtx == nil {
		return ""
	}
	runID := ""
	if execCtx.Engine != nil {
		runID = strings.TrimSpace(execCtx.Engine.Options.RunID)
	}
	logsRoot := strings.TrimSpace(execCtx.LogsRoot)
	worktree := strings.TrimSpace(execCtx.WorktreeDir)
	stageDir := ""
	if logsRoot != "" && strings.TrimSpace(nodeID) != "" {
		stageDir = filepath.Join(logsRoot, strings.TrimSpace(nodeID))
	}
	if runID == "" && logsRoot == "" && stageDir == "" && worktree == "" && strings.TrimSpace(nodeID) == "" {
		return ""
	}
	return strings.TrimSpace(
		"Execution context:\n" +
			"- $" + runIDEnvKey + "=" + runID + "\n" +
			"- $" + logsRootEnvKey + "=" + logsRoot + "\n" +
			"- $" + stageLogsDirEnvKey + "=" + stageDir + "\n" +
			"- $" + worktreeDirEnvKey + "=" + worktree + "\n" +
			"- $" + nodeIDEnvKey + "=" + strings.TrimSpace(nodeID) + "\n",
	)
}

// buildAgentLoopOverrides extracts the subset of base-node environment
// invariants needed by the API agent_loop path and merges contract env vars.
// It bridges buildBaseNodeEnv's []string format to agent.BaseEnv's map format.
func buildAgentLoopOverrides(worktreeDir string, rp ResolvedArtifactPolicy, contractEnv map[string]string) map[string]string {
	base := buildBaseNodeEnv(worktreeDir, rp)
	normalized := normalizeResolvedArtifactPolicy(rp)
	keep := make(map[string]bool, len(normalized.Env.Vars))
	for k := range normalized.Env.Vars {
		keep[k] = true
	}
	out := make(map[string]string, len(contractEnv)+len(keep))
	for _, kv := range base {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if keep[k] {
			out[k] = v
		}
	}
	for k, v := range contractEnv {
		out[k] = v
	}
	return out
}

func artifactPolicyFromExecution(execCtx *Execution) ResolvedArtifactPolicy {
	if execCtx == nil || execCtx.Engine == nil {
		return ResolvedArtifactPolicy{}
	}
	return execCtx.Engine.ArtifactPolicy
}
