package main

import (
	"log"
	"os"

	"github.com/streadway/amqp"

	"go-commerce/internal/notification"
	"go-commerce/pkg/events"
	"go-commerce/pkg/mq"
)

const notificationQueueName = "notification.order.created"

func main() {
	rabbitmqURL := os.Getenv("RABBITMQ_URL")
	if rabbitmqURL == "" {
		rabbitmqURL = "amqp://guest:guest@rabbitmq:5672/"
	}

	exchangeName := os.Getenv("EVENT_EXCHANGE")
	if exchangeName == "" {
		exchangeName = mq.DefaultExchangeName
	}

	conn, err := amqp.Dial(rabbitmqURL)
	if err != nil {
		log.Fatalf("rabbitmq_connection_failed error=%v", err)
	}
	defer conn.Close()

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

	consumer := notification.NewConsumer(log.Default())
	log.Printf("notification_service_started exchange=%s queue=%s routing_key=%s", exchangeName, queue.Name, events.OrderCreatedType)

	for delivery := range deliveries {
		if err := consumer.HandleDelivery(delivery); err != nil {
			log.Printf("notification_event_handle_failed error=%v", err)
		}
	}
}
