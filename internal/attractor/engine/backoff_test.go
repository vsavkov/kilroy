package engine

import (
	"testing"
	"time"

	"github.com/danshapiro/kilroy/internal/attractor/model"
)

func TestDelayForAttempt_NoJitter_ConstantFactorOne(t *testing.T) {
	cfg := BackoffConfig{
		InitialDelayMS: 10,
		BackoffFactor:  1.0,
		MaxDelayMS:     1000,
		Jitter:         false,
	}
	for attempt := 1; attempt <= 5; attempt++ {
		if got := DelayForAttempt(attempt, cfg, "seed"); got != 10*time.Millisecond {
			t.Fatalf("attempt %d: got %v want %v", attempt, got, 10*time.Millisecond)
		}
	}
}

func TestDelayForAttempt_NoJitter_ExponentialAndCapped(t *testing.T) {
	cfg := BackoffConfig{
		InitialDelayMS: 50,
		BackoffFactor:  10.0,
		MaxDelayMS:     200,
		Jitter:         false,
	}
	if got := DelayForAttempt(1, cfg, "seed"); got != 50*time.Millisecond {
		t.Fatalf("attempt 1: got %v want %v", got, 50*time.Millisecond)
	}
	// 50 * 10 = 500ms but capped at 200ms (before jitter).
	if got := DelayForAttempt(2, cfg, "seed"); got != 200*time.Millisecond {
		t.Fatalf("attempt 2: got %v want %v", got, 200*time.Millisecond)
	}
	// Still capped.
	if got := DelayForAttempt(3, cfg, "seed"); got != 200*time.Millisecond {
		t.Fatalf("attempt 3: got %v want %v", got, 200*time.Millisecond)
	}
}

func TestDelayForAttempt_Jitter_IsDeterministicPerSeedAndWithinRange(t *testing.T) {
	cfg := BackoffConfig{
		InitialDelayMS: 100,
		BackoffFactor:  1.0,
		MaxDelayMS:     1000,
		Jitter:         true,
	}
	d1 := DelayForAttempt(1, cfg, "seed-a")
	d1b := DelayForAttempt(1, cfg, "seed-a")
	if d1 != d1b {
		t.Fatalf("expected deterministic delay for same seed: %v vs %v", d1, d1b)
	}
	min := 50 * time.Millisecond
	max := 150 * time.Millisecond
	if d1 < min || d1 > max {
		t.Fatalf("delay out of jitter range: got %v want within [%v, %v]", d1, min, max)
	}
	d2 := DelayForAttempt(1, cfg, "seed-b")
	if d2 == d1 {
		t.Fatalf("expected different seed to produce different jittered delay (got %v)", d2)
	}
	if d2 < min || d2 > max {
		t.Fatalf("delay out of jitter range: got %v want within [%v, %v]", d2, min, max)
	}
}

func TestBackoffConfigFor_ParsesGraphAndNodeOverrides(t *testing.T) {
	g := model.NewGraph("g")
	g.Attrs["retry.backoff.initial_delay_ms"] = "10"
	g.Attrs["retry.backoff.backoff_factor"] = "1"
	g.Attrs["retry.backoff.max_delay_ms"] = "1000"
	g.Attrs["retry.backoff.jitter"] = "false"

	n := model.NewNode("n")
	cfg := backoffConfigFor(g, n)
	if cfg.InitialDelayMS != 10 || cfg.BackoffFactor != 1.0 || cfg.MaxDelayMS != 1000 || cfg.Jitter != false {
		t.Fatalf("graph cfg: %+v", cfg)
	}

	// Node overrides win.
	n.Attrs["retry.backoff.initial_delay_ms"] = "25"
	n.Attrs["retry.backoff.jitter"] = "true"
	cfg = backoffConfigFor(g, n)
	if cfg.InitialDelayMS != 25 {
		t.Fatalf("node initial_delay override: got %d want %d", cfg.InitialDelayMS, 25)
	}
	if cfg.Jitter != true {
		t.Fatalf("node jitter override: got %v want %v", cfg.Jitter, true)
	}

	// backoffDelayForNode uses these settings.
	d := backoffDelayForNode("run", g, n, 1)
	if d < 12*time.Millisecond || d > 38*time.Millisecond {
		t.Fatalf("expected jittered 25ms delay within [12ms, 38ms], got %v", d)
	}
}
