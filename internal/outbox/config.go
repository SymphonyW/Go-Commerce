package outbox

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

const (
	DefaultPollInterval   = 5 * time.Second
	DefaultBatchSize      = 100
	DefaultMaxRetry       = 5
	DefaultRetryBaseDelay = time.Second
)

type Config struct {
	PollInterval   time.Duration
	BatchSize      int
	MaxRetry       int
	RetryBaseDelay time.Duration
}

func LoadConfigFromEnv() (Config, error) {
	cfg := Config{
		PollInterval:   DefaultPollInterval,
		BatchSize:      DefaultBatchSize,
		MaxRetry:       DefaultMaxRetry,
		RetryBaseDelay: DefaultRetryBaseDelay,
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
