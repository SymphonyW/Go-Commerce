package outbox

import (
	"strings"
	"testing"
	"time"
)

func TestLoadConfigFromEnvIncludesLeaseAndWorkerID(t *testing.T) {
	t.Setenv("OUTBOX_POLL_INTERVAL", "2s")
	t.Setenv("OUTBOX_BATCH_SIZE", "25")
	t.Setenv("OUTBOX_MAX_RETRY", "3")
	t.Setenv("OUTBOX_RETRY_BASE_DELAY", "500ms")
	t.Setenv("OUTBOX_LEASE_DURATION", "45s")
	t.Setenv("OUTBOX_WORKER_ID", "worker-from-env")

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv returned error: %v", err)
	}

	if got, want := cfg.PollInterval, 2*time.Second; got != want {
		t.Fatalf("unexpected poll interval: got %s want %s", got, want)
	}
	if got, want := cfg.BatchSize, 25; got != want {
		t.Fatalf("unexpected batch size: got %d want %d", got, want)
	}
	if got, want := cfg.MaxRetry, 3; got != want {
		t.Fatalf("unexpected max retry: got %d want %d", got, want)
	}
	if got, want := cfg.RetryBaseDelay, 500*time.Millisecond; got != want {
		t.Fatalf("unexpected retry base delay: got %s want %s", got, want)
	}
	if got, want := cfg.LeaseDuration, 45*time.Second; got != want {
		t.Fatalf("unexpected lease duration: got %s want %s", got, want)
	}
	if got, want := cfg.WorkerID, "worker-from-env"; got != want {
		t.Fatalf("unexpected worker id: got %q want %q", got, want)
	}
}

func TestConfigDefaultsGenerateWorkerID(t *testing.T) {
	cfg := (Config{}).withDefaults()
	if cfg.WorkerID == "" {
		t.Fatal("expected generated worker id")
	}
	if !strings.Contains(cfg.WorkerID, "-") {
		t.Fatalf("expected generated worker id to include hostname and uuid suffix, got %q", cfg.WorkerID)
	}
	if got, want := cfg.LeaseDuration, DefaultLeaseDuration; got != want {
		t.Fatalf("unexpected lease duration default: got %s want %s", got, want)
	}
}
