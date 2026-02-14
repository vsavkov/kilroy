package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/danshapiro/kilroy/internal/cxdb"
)

// ResumeFromCXDB resumes a run by reading the latest checkpoint pointer from the CXDB context head.
//
// This supports the metaspec requirement: resume MUST be possible from the CXDB trajectory.
func ResumeFromCXDB(ctx context.Context, cxdbHTTPBaseURL string, contextID string) (*Result, error) {
	cxdbHTTPBaseURL = strings.TrimSpace(cxdbHTTPBaseURL)
	contextID = strings.TrimSpace(contextID)
	if cxdbHTTPBaseURL == "" || contextID == "" {
		return nil, fmt.Errorf("cxdb_http_base_url and context_id are required")
	}
	c := cxdb.New(cxdbHTTPBaseURL)
	if err := c.Health(ctx); err != nil {
		return nil, err
	}

	turns, err := c.ListTurns(ctx, contextID, cxdb.ListTurnsOptions{Limit: 500})
	if err != nil {
		return nil, err
	}
	checkpointPath := ""
	checkpointDepth := -1
	logsRoot := ""
	logsDepth := -1
	for _, t := range turns {
		switch strings.TrimSpace(t.TypeID) {
		case "com.kilroy.attractor.CheckpointSaved":
			if t.Payload == nil {
				continue
			}
			if v, ok := t.Payload["checkpoint_path"]; ok {
				p := strings.TrimSpace(fmt.Sprint(v))
				if p != "" && t.Depth >= checkpointDepth {
					checkpointDepth = t.Depth
					checkpointPath = p
				}
			}
		case "com.kilroy.attractor.RunStarted":
			if t.Payload == nil {
				continue
			}
			if v, ok := t.Payload["logs_root"]; ok {
				p := strings.TrimSpace(fmt.Sprint(v))
				if p != "" && t.Depth >= logsDepth {
					logsDepth = t.Depth
					logsRoot = p
				}
			}
		}
	}
	if checkpointPath != "" {
		logsRoot = filepath.Dir(checkpointPath)
	}
	if strings.TrimSpace(logsRoot) == "" {
		return nil, fmt.Errorf("cxdb context %s: could not find checkpoint_path or logs_root in recent turns", contextID)
	}

	return resumeFromLogsRoot(ctx, logsRoot, ResumeOverrides{
		CXDBHTTPBaseURL: cxdbHTTPBaseURL,
		CXDBContextID:   contextID,
	})
}

// ResumeFromBranch resumes a run given only the git run branch name, using best-effort discovery
// of the logs_root in the default state directory.
func ResumeFromBranch(ctx context.Context, repoPath string, runBranch string) (*Result, error) {
	_ = repoPath // manifest is authoritative; repoPath is kept for future override support
	runBranch = strings.TrimSpace(runBranch)
	if runBranch == "" {
		return nil, fmt.Errorf("run_branch is required")
	}

	// Common case: branch name ends with the run_id.
	runID := filepath.Base(runBranch)
	guess := defaultLogsRoot(runID)
	if _, err := os.Stat(filepath.Join(guess, "manifest.json")); err == nil {
		return resumeFromLogsRoot(ctx, guess, ResumeOverrides{})
	}

	// Best-effort scan of the default runs directory for a manifest that matches run_branch.
	runsDir := filepath.Dir(guess) // .../kilroy/attractor/runs
	entries, err := os.ReadDir(runsDir)
	if err == nil {
		for _, ent := range entries {
			if !ent.IsDir() {
				continue
			}
			logsRoot := filepath.Join(runsDir, ent.Name())
			m, err := loadManifest(filepath.Join(logsRoot, "manifest.json"))
			if err != nil {
				continue
			}
			if strings.TrimSpace(m.RunBranch) == runBranch {
				return resumeFromLogsRoot(ctx, logsRoot, ResumeOverrides{})
			}
		}
	}
	return nil, fmt.Errorf("could not locate logs_root for run_branch %q (tried %s and scanned %s)", runBranch, guess, runsDir)
}
