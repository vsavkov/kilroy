package engine

import "testing"

func TestDefaultCLIInvocation_GoogleGeminiNonInteractive(t *testing.T) {
	exe, args := defaultCLIInvocation("google", "gemini-3-flash-preview", "/tmp/worktree")
	if exe == "" {
		t.Fatalf("expected non-empty executable for google")
	}
	if !hasArg(args, "-p") {
		t.Fatalf("expected -p in args (headless prompt mode); args=%v", args)
	}
	// Spec/metaspec: CLI adapters must not block on interactive approvals.
	if !hasArg(args, "--yolo") {
		t.Fatalf("expected --yolo in args to force non-interactive approvals; args=%v", args)
	}
	// Ensure we pass the model explicitly.
	foundModel := false
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--model" && args[i+1] == "gemini-3-flash-preview" {
			foundModel = true
			break
		}
	}
	if !foundModel {
		t.Fatalf("expected --model gemini-3-flash-preview in args; args=%v", args)
	}
}

func TestDefaultCLIInvocation_OpenAI_DoesNotUseDeprecatedAskForApproval(t *testing.T) {
	exe, args := defaultCLIInvocation("openai", "gpt-5.3-codex", "/tmp/worktree")
	if exe == "" {
		t.Fatalf("expected non-empty executable for openai")
	}
	if hasArg(args, "--ask-for-approval") {
		t.Fatalf("unexpected deprecated --ask-for-approval flag: %v", args)
	}
	if !hasArg(args, "--json") {
		t.Fatalf("expected --json: %v", args)
	}
}

func TestDefaultCLIInvocation_Anthropic_VerboseCapabilityGate(t *testing.T) {
	router := NewCodergenRouter(nil, nil)
	router.SetCLICapabilities(map[string]providerCLICapabilities{
		"anthropic": {SupportsVerbose: true},
	})
	_, verboseArgs := router.cliInvocation("anthropic", "claude-3-7-sonnet", "/tmp/worktree")
	if !hasArg(verboseArgs, "--verbose") {
		t.Fatalf("expected --verbose when preflight indicates support; args=%v", verboseArgs)
	}

	router.SetCLICapabilities(map[string]providerCLICapabilities{
		"anthropic": {SupportsVerbose: false},
	})
	_, noVerboseArgs := router.cliInvocation("anthropic", "claude-3-7-sonnet", "/tmp/worktree")
	if hasArg(noVerboseArgs, "--verbose") {
		t.Fatalf("did not expect --verbose when preflight indicates unsupported; args=%v", noVerboseArgs)
	}
}
