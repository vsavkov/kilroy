package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/strongdm/kilroy/internal/attractor/gitutil"
	"github.com/strongdm/kilroy/internal/attractor/runtime"
)

// runSubgraphUntil executes a subgraph starting at startNodeID and stops when the next hop would enter stopNodeID.
// The stop node itself is not executed. This is used to run parallel branches up to a shared fan-in node.
func runSubgraphUntil(ctx context.Context, eng *Engine, startNodeID, stopNodeID string) (parallelBranchResult, error) {
	if eng == nil || eng.Graph == nil {
		return parallelBranchResult{}, fmt.Errorf("subgraph engine is nil")
	}
	if strings.TrimSpace(startNodeID) == "" {
		return parallelBranchResult{}, fmt.Errorf("start node is required")
	}

	headSHA, _ := gitutil.HeadSHA(eng.WorktreeDir)

	current := startNodeID
	completed := []string{}
	nodeRetries := map[string]int{}

	var lastNode string
	var lastOutcome runtime.Outcome
	emitCanceledExit := func(nodeID string, out runtime.Outcome) {
		eng.appendProgress(map[string]any{
			"event":          "subgraph_canceled_exit",
			"node_id":        strings.TrimSpace(nodeID),
			"last_node_id":   strings.TrimSpace(lastNode),
			"failure_reason": strings.TrimSpace(out.FailureReason),
		})
	}
	buildResult := func(out runtime.Outcome) parallelBranchResult {
		return parallelBranchResult{
			HeadSHA:    headSHA,
			LastNodeID: lastNode,
			Outcome:    out,
			Completed:  completed,
		}
	}
	canceledReturn := func(nodeID string, out runtime.Outcome, cause error) (parallelBranchResult, error) {
		emitCanceledExit(nodeID, out)
		return buildResult(out), cause
	}

	for {
		if err := ctx.Err(); err != nil {
			return canceledReturn(current, lastOutcome, err)
		}

		if strings.TrimSpace(stopNodeID) != "" && current == stopNodeID {
			return parallelBranchResult{
				HeadSHA:    headSHA,
				LastNodeID: lastNode,
				Outcome:    lastOutcome,
				Completed:  completed,
			}, nil
		}

		node := eng.Graph.Nodes[current]
		if node == nil {
			return parallelBranchResult{}, fmt.Errorf("missing node: %s", current)
		}

		eng.cxdbStageStarted(ctx, node)
		out, err := eng.executeWithRetry(ctx, node, nodeRetries)
		if err != nil {
			return parallelBranchResult{}, err
		}
		eng.cxdbStageFinished(ctx, node, out)
		if err := ctx.Err(); err != nil {
			return canceledReturn(node.ID, out, err)
		}

		// Record completion.
		completed = append(completed, node.ID)

		// Apply context updates and built-ins.
		eng.Context.ApplyUpdates(out.ContextUpdates)
		eng.Context.Set("outcome", string(out.Status))
		eng.Context.Set("preferred_label", out.PreferredLabel)
		eng.Context.Set("failure_reason", out.FailureReason)
		failureClass := classifyFailureClass(out)
		eng.Context.Set("failure_class", failureClass)

		if isFailureLoopRestartOutcome(out) && normalizedFailureClassOrDefault(failureClass) == failureClassDeterministic {
			sig := restartFailureSignature(node.ID, out, failureClass)
			if sig != "" {
				if eng.loopFailureSignatures == nil {
					eng.loopFailureSignatures = map[string]int{}
				}
				eng.loopFailureSignatures[sig]++
				count := eng.loopFailureSignatures[sig]
				limit := loopRestartSignatureLimit(eng.Graph)
				eng.appendProgress(map[string]any{
					"event":           "subgraph_deterministic_failure_cycle_check",
					"node_id":         node.ID,
					"signature":       sig,
					"signature_count": count,
					"signature_limit": limit,
					"failure_class":   failureClass,
					"failure_reason":  out.FailureReason,
				})
				if count >= limit {
					eng.appendProgress(map[string]any{
						"event":           "subgraph_deterministic_failure_cycle_breaker",
						"node_id":         node.ID,
						"signature":       sig,
						"signature_count": count,
						"signature_limit": limit,
					})
					return parallelBranchResult{
						HeadSHA:    headSHA,
						LastNodeID: lastNode,
						Outcome:    out,
						Completed:  completed,
					}, fmt.Errorf("deterministic failure cycle detected in subgraph: %s", sig)
				}
			}
		} else if out.Status == runtime.StatusSuccess {
			eng.loopFailureSignatures = nil
		}

		sha, err := eng.checkpoint(node.ID, out, completed, nodeRetries)
		if err != nil {
			return parallelBranchResult{}, err
		}
		eng.cxdbCheckpointSaved(ctx, node.ID, out.Status, sha)
		headSHA = sha
		lastNode = node.ID
		lastOutcome = out
		if err := ctx.Err(); err != nil {
			return canceledReturn(node.ID, lastOutcome, err)
		}

		next, err := selectNextEdge(eng.Graph, node.ID, out, eng.Context)
		if err != nil {
			return parallelBranchResult{}, err
		}
		if next == nil {
			return parallelBranchResult{
				HeadSHA:    headSHA,
				LastNodeID: lastNode,
				Outcome:    lastOutcome,
				Completed:  completed,
			}, nil
		}
		if strings.TrimSpace(stopNodeID) != "" && next.To == stopNodeID {
			return parallelBranchResult{
				HeadSHA:    headSHA,
				LastNodeID: lastNode,
				Outcome:    lastOutcome,
				Completed:  completed,
			}, nil
		}
		if strings.EqualFold(next.Attr("loop_restart", "false"), "true") {
			return parallelBranchResult{}, fmt.Errorf("loop_restart not supported in v1")
		}
		if err := ctx.Err(); err != nil {
			return canceledReturn(node.ID, lastOutcome, err)
		}
		current = next.To
	}
}
