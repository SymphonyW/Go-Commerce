package main

import (
	"log"
	"time"

	"github.com/streadway/amqp"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"go-commerce/internal/inbox"
	"go-commerce/internal/notification"
	"go-commerce/pkg/events"
	"go-commerce/pkg/healthcheck"
	"go-commerce/pkg/mq"
	"go-commerce/pkg/serviceutil"
)

const notificationQueueName = "notification.order.created"

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

	if err := db.AutoMigrate(&inbox.ConsumedEvent{}); err != nil {
		log.Fatalf("mysql_migrate_failed error=%v", err)
	}

	exchangeName := serviceutil.Env("EVENT_EXCHANGE", mq.DefaultExchangeName)
	conn, err := amqp.Dial(serviceutil.Env("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"))
	if err != nil {
		log.Fatalf("rabbitmq_connection_failed error=%v", err)
	}
	defer conn.Close()
	log.Printf("rabbitmq_connected")

	channel, err := conn.Channel()
	if err != nil {
		log.Fatalf("rabbitmq_channel_open_failed error=%v", err)
	}
	defer channel.Close()

	if err := channel.ExchangeDeclare(exchangeName, "topic", true, false, false, false, nil); err != nil {
		log.Fatalf("rabbitmq_exchange_declare_failed exchange=%s error=%v", exchangeName, err)
	}
	queue, err := channel.QueueDeclare(notificationQueueName, true, false, false, false, nil)
	if err != nil {
		log.Fatalf("rabbitmq_queue_declare_failed queue=%s error=%v", notificationQueueName, err)
	}
	if err := channel.QueueBind(queue.Name, events.OrderCreatedType, exchangeName, false, nil); err != nil {
		log.Fatalf("rabbitmq_queue_bind_failed queue=%s routing_key=%s error=%v", queue.Name, events.OrderCreatedType, err)
	}
	deliveries, err := channel.Consume(queue.Name, "", false, false, false, false, nil)
	if err != nil {
		log.Fatalf("rabbitmq_consume_failed queue=%s error=%v", queue.Name, err)
	}

	healthServer := serviceutil.StartHTTPServer(
		"notification health server",
		serviceutil.Env("NOTIFICATION_HEALTH_ADDR", ":8087"),
		healthcheck.Handler(
			healthcheck.Dependency{Name: "mysql", Check: healthcheck.SQL(sqlDB)},
			healthcheck.Dependency{Name: "rabbitmq", Check: healthcheck.AMQP(conn)},
		),
	)

	consumer := notification.NewConsumer(db, log.Default())
	log.Printf("notification service started exchange=%s queue=%s routing_key=%s", exchangeName, queue.Name, events.OrderCreatedType)

	for {
		select {
		case <-ctx.Done():
			log.Printf("shutdown_started signal=%v", ctx.Err())
			serviceutil.ShutdownHTTPServer(healthServer, 5*time.Second)
			log.Printf("notification service shutdown completed")
			return
		case delivery, ok := <-deliveries:
			if !ok {
				log.Printf("rabbitmq_delivery_channel_closed")
				serviceutil.ShutdownHTTPServer(healthServer, 5*time.Second)
				return
			}
			if err := consumer.HandleDelivery(delivery); err != nil {
				log.Printf("notification_event_handle_failed error=%v", err)
			}
		}
	}
}
