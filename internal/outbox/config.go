package outbox

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultPollInterval   = 5 * time.Second
	DefaultBatchSize      = 100
	DefaultMaxRetry       = 5
	DefaultRetryBaseDelay = time.Second
	DefaultLeaseDuration  = 30 * time.Second
)

type Config struct {
	PollInterval   time.Duration
	BatchSize      int
	MaxRetry       int
	RetryBaseDelay time.Duration
	LeaseDuration  time.Duration
	WorkerID       string
}

func LoadConfigFromEnv() (Config, error) {
	cfg := Config{
		PollInterval:   DefaultPollInterval,
		BatchSize:      DefaultBatchSize,
		MaxRetry:       DefaultMaxRetry,
		RetryBaseDelay: DefaultRetryBaseDelay,
		LeaseDuration:  DefaultLeaseDuration,
		WorkerID:       strings.TrimSpace(os.Getenv("OUTBOX_WORKER_ID")),
	}

	var err error
	if raw := os.Getenv("OUTBOX_POLL_INTERVAL"); raw != "" {
		cfg.PollInterval, err = time.ParseDuration(raw)
		if err != nil || cfg.PollInterval <= 0 {
			return Config{}, fmt.Errorf("invalid OUTBOX_POLL_INTERVAL: %q", raw)
		}
	}
	if raw := os.Getenv("OUTBOX_BATCH_SIZE"); raw != "" {
		cfg.BatchSize, err = strconv.Atoi(raw)
		if err != nil || cfg.BatchSize <= 0 {
			return Config{}, fmt.Errorf("invalid OUTBOX_BATCH_SIZE: %q", raw)
		}
	}
	if raw := os.Getenv("OUTBOX_MAX_RETRY"); raw != "" {
		cfg.MaxRetry, err = strconv.Atoi(raw)
		if err != nil || cfg.MaxRetry <= 0 {
			return Config{}, fmt.Errorf("invalid OUTBOX_MAX_RETRY: %q", raw)
		}
	}
	if raw := os.Getenv("OUTBOX_RETRY_BASE_DELAY"); raw != "" {
		cfg.RetryBaseDelay, err = time.ParseDuration(raw)
		if err != nil || cfg.RetryBaseDelay <= 0 {
			return Config{}, fmt.Errorf("invalid OUTBOX_RETRY_BASE_DELAY: %q", raw)
		}
	}
	if raw := os.Getenv("OUTBOX_LEASE_DURATION"); raw != "" {
		cfg.LeaseDuration, err = time.ParseDuration(raw)
		if err != nil || cfg.LeaseDuration <= 0 {
			return Config{}, fmt.Errorf("invalid OUTBOX_LEASE_DURATION: %q", raw)
		}
	}
	if cfg.WorkerID == "" {
		cfg.WorkerID = defaultWorkerID()
	}

	return cfg, nil
}

func (c Config) withDefaults() Config {
	if c.PollInterval <= 0 {
		c.PollInterval = DefaultPollInterval
	}
	if c.BatchSize <= 0 {
		c.BatchSize = DefaultBatchSize
	}
	if c.MaxRetry <= 0 {
		c.MaxRetry = DefaultMaxRetry
	}
	if c.RetryBaseDelay <= 0 {
		c.RetryBaseDelay = DefaultRetryBaseDelay
	}
	if c.LeaseDuration <= 0 {
		c.LeaseDuration = DefaultLeaseDuration
	}
	if strings.TrimSpace(c.WorkerID) == "" {
		c.WorkerID = defaultWorkerID()
	} else {
		c.WorkerID = strings.TrimSpace(c.WorkerID)
	}
	return c
}

// RetryDelay 使用固定阶梯的指数退避：1s、5s、30s、1m、5m。
// 当配置了不同基准值时，阶梯仍按相同比例缩放。
func (c Config) RetryDelay(retryCount int) time.Duration {
	c = c.withDefaults()
	multipliers := []time.Duration{1, 5, 30, 60, 300}
	if retryCount <= 0 {
		return c.RetryBaseDelay
	}
	index := retryCount - 1
	if index >= len(multipliers) {
		index = len(multipliers) - 1
	}
	return c.RetryBaseDelay * multipliers[index]
}

func defaultWorkerID() string {
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		hostname = "unknown-host"
	}
	return fmt.Sprintf("%s-%s", hostname, newUUIDString())
}

func newUUIDString() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UTC().UnixNano(), 36)
	}

	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	encoded := hex.EncodeToString(b[:])
	return fmt.Sprintf(
		"%s-%s-%s-%s-%s",
		encoded[0:8],
		encoded[8:12],
		encoded[12:16],
		encoded[16:20],
		encoded[20:32],
	)
}
