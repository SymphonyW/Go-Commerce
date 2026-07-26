package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/streadway/amqp"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"go-commerce/internal/outbox"
	"go-commerce/pkg/healthcheck"
	"go-commerce/pkg/mq"
	"go-commerce/pkg/serviceutil"
)

func main() {
	ctx, stop := serviceutil.SignalContext()
	defer stop()

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

	if err := db.AutoMigrate(&outbox.Event{}); err != nil {
		log.Fatalf("mysql_migrate_failed error=%v", err)
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
	worker := outbox.NewWorker(repo, mq.NewRabbitMQPublisher(channel, exchangeName), config, log.Default())
	registry := prometheus.NewRegistry()
	worker.SetMetrics(outbox.NewPrometheusMetrics("outbox-worker", registry))

	probeHandler := healthcheck.Handler(
		healthcheck.Dependency{Name: "mysql", Check: healthcheck.SQL(sqlDB)},
		healthcheck.Dependency{Name: "rabbitmq", Check: healthcheck.AMQP(conn)},
		healthcheck.Dependency{Name: "outbox_worker_polling", Check: worker.CheckPolling},
	)
	httpMux := http.NewServeMux()
	httpMux.Handle("/healthz", probeHandler)
	httpMux.Handle("/readyz", probeHandler)
	httpMux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))

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
