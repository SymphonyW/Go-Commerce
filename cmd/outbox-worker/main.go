package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/streadway/amqp"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"go-commerce/internal/outbox"
	"go-commerce/pkg/healthcheck"
	"go-commerce/pkg/mq"
	"go-commerce/pkg/observability"
	"go-commerce/pkg/serviceutil"
)

func main() {
	ctx, stop := serviceutil.SignalContext()
	defer stop()
	telemetry := observability.SetupService(ctx, "outbox-worker")
	logger := telemetry.Logger
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := telemetry.Shutdown(shutdownCtx); err != nil {
			logger.Error("otel_shutdown_failed", "error", err)
		}
	}()

	dsn := serviceutil.Env("DB_DSN", "root:password@tcp(127.0.0.1:3307)/ecommerce?charset=utf8mb4&parseTime=True&loc=Local")
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("mysql_connect_failed error=%v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("mysql_handle_failed error=%v", err)
	}
	defer sqlDB.Close()
	log.Printf("mysql_connected")

	if serviceutil.AutoMigrateEnabled() {
		log.Printf("auto_migrate_enabled warning=use_cmd_migrate_for_shared_mysql")
		if err := db.AutoMigrate(&outbox.Event{}); err != nil {
			log.Fatalf("mysql_migrate_failed error=%v", err)
		}
	} else {
		log.Printf("auto_migrate_disabled command=\"go run ./cmd/migrate up\"")
	}
	config, err := outbox.LoadConfigFromEnv()
	if err != nil {
		log.Fatalf("invalid_outbox_config error=%v", err)
	}

	exchangeName := serviceutil.Env("EVENT_EXCHANGE", mq.DefaultExchangeName)
	conn, err := amqp.Dial(serviceutil.Env("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"))
	if err != nil {
		log.Fatalf("rabbitmq_connect_failed error=%v", err)
	}
	defer conn.Close()
	log.Printf("rabbitmq_connected")

	channel, err := conn.Channel()
	if err != nil {
		log.Fatalf("rabbitmq_channel_open_failed error=%v", err)
	}
	defer channel.Close()

	repo := outbox.NewRepository(db)
	publisher := mq.NewInstrumentedPublisher(mq.NewRabbitMQPublisher(channel, exchangeName), telemetry.Metrics)
	worker := outbox.NewWorker(repo, publisher, config, log.Default())
	worker.SetMetrics(outbox.NewPrometheusMetrics("outbox-worker", telemetry.Registry))
	telemetry.Metrics.RegisterOutboxPendingGauge(telemetry.Registry, func() float64 {
		return countPendingOutbox(db)
	})
	telemetry.Metrics.RegisterOutboxOldestPendingGauge(telemetry.Registry, func() float64 {
		return oldestPendingOutboxAge(db, time.Now())
	})

	probeHandler := healthcheck.Handler(
		healthcheck.Dependency{Name: "mysql", Check: healthcheck.SQL(sqlDB)},
		healthcheck.Dependency{Name: "rabbitmq", Check: healthcheck.AMQP(conn)},
		healthcheck.Dependency{Name: "outbox_worker_polling", Check: worker.CheckPolling},
	)
	httpMux := http.NewServeMux()
	httpMux.Handle("/healthz", probeHandler)
	httpMux.Handle("/readyz", probeHandler)
	httpMux.Handle("/metrics", promhttp.HandlerFor(telemetry.Registry, promhttp.HandlerOpts{}))

	healthServer := serviceutil.StartHTTPServer(
		"outbox health server",
		serviceutil.Env("OUTBOX_HEALTH_ADDR", ":8088"),
		httpMux,
	)
	defer serviceutil.ShutdownHTTPServer(healthServer, 5*time.Second)

	log.Printf(
		"outbox worker started worker_id=%s poll_interval=%s batch_size=%d max_retry=%d retry_base_delay=%s lease_duration=%s",
		worker.WorkerID(),
		config.PollInterval,
		config.BatchSize,
		config.MaxRetry,
		config.RetryBaseDelay,
		config.LeaseDuration,
	)
	if err := worker.Run(ctx); err != nil && err != context.Canceled {
		log.Fatalf("outbox_worker_stopped_unexpectedly error=%v", err)
	}
	log.Printf("outbox worker shutdown completed")
}

func countPendingOutbox(db *gorm.DB) float64 {
	if db == nil {
		return 0
	}
	var count int64
	if err := db.Model(&outbox.Event{}).Where("status = ?", outbox.StatusPending).Count(&count).Error; err != nil {
		return 0
	}
	return float64(count)
}

func oldestPendingOutboxAge(db *gorm.DB, now time.Time) float64 {
	if db == nil {
		return 0
	}
	var event outbox.Event
	if err := db.Where("status = ?", outbox.StatusPending).Order("created_at ASC").First(&event).Error; err != nil {
		return 0
	}
	if event.CreatedAt.IsZero() || now.Before(event.CreatedAt) {
		return 0
	}
	return now.Sub(event.CreatedAt).Seconds()
}
