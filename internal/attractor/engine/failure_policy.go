package engine

import "github.com/danshapiro/kilroy/internal/attractor/runtime"

// retryableFailureClasses lists failure classes that should trigger automatic retries.
// transient_infra: temporary infrastructure issues (API timeouts, rate limits)
// budget_exhausted: model ran out of turn/token budget (may succeed with retry or escalation)
// compilation_loop: model stuck in fix-regress cycle (may succeed with different approach on retry)
var retryableFailureClasses = map[string]bool{
	failureClassTransientInfra:  true,
	failureClassBudgetExhausted: true,
	failureClassCompilationLoop: true,
}

func shouldRetryOutcome(out runtime.Outcome, failureClass string) bool {
	if out.Status != runtime.StatusFail && out.Status != runtime.StatusRetry {
		return false
	}
	return retryableFailureClasses[normalizedFailureClassOrDefault(failureClass)]
}

// isEscalatableFailureClass returns true if the failure class should trigger
// model escalation (as opposed to same-model retry for transient issues).
func isEscalatableFailureClass(failureClass string) bool {
	cls := normalizedFailureClassOrDefault(failureClass)
	return cls == failureClassBudgetExhausted || cls == failureClassCompilationLoop
}
