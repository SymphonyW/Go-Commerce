package observability

import (
	"context"
	"log"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
)

type ServiceTelemetry struct {
	Service  string
	Logger   *slog.Logger
	Registry *prometheus.Registry
	Metrics  *Metrics
	Shutdown TraceShutdown
}

func SetupService(ctx context.Context, service string) ServiceTelemetry {
	logger := NewLogger(service)
	slog.SetDefault(logger)
	log.SetFlags(0)
	log.SetOutput(NewLogWriter(logger))

	registry := prometheus.NewRegistry()
	metrics := NewMetrics(service, registry)
	shutdown := InitTracing(ctx, service, logger)
	return ServiceTelemetry{
		Service:  service,
		Logger:   logger,
		Registry: registry,
		Metrics:  metrics,
		Shutdown: shutdown,
	}
}
