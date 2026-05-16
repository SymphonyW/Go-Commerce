package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/streadway/amqp"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"go-commerce/internal/outbox"
	"go-commerce/pkg/mq"
)

func main() {
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		dsn = "root:password@tcp(127.0.0.1:3307)/ecommerce?charset=utf8mb4&parseTime=True&loc=Local"
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}
	if err := db.AutoMigrate(&outbox.Event{}); err != nil {
		log.Fatalf("failed to migrate outbox table: %v", err)
	}

	config, err := outbox.LoadConfigFromEnv()
	if err != nil {
		log.Fatalf("invalid outbox config: %v", err)
	}

	rabbitmqURL := os.Getenv("RABBITMQ_URL")
	if rabbitmqURL == "" {
		rabbitmqURL = "amqp://guest:guest@localhost:5672/"
	}
	exchangeName := os.Getenv("EVENT_EXCHANGE")
	if exchangeName == "" {
		exchangeName = mq.DefaultExchangeName
	}

	conn, err := amqp.Dial(rabbitmqURL)
	if err != nil {
		log.Fatalf("failed to connect rabbitmq: %v", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("failed to open rabbitmq channel: %v", err)
	}
	defer ch.Close()

	repo := outbox.NewRepository(db)
	worker := outbox.NewWorker(repo, mq.NewRabbitMQPublisher(ch, exchangeName), config, log.Default())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf(
		"outbox_worker_started poll_interval=%s batch_size=%d max_retry=%d retry_base_delay=%s",
		config.PollInterval,
		config.BatchSize,
		config.MaxRetry,
		config.RetryBaseDelay,
	)
	if err := worker.Run(ctx); err != nil && err != context.Canceled {
		log.Fatalf("outbox worker stopped unexpectedly: %v", err)
	}
}
