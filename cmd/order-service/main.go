package main

import (
	"log"
	"net"
	"time"

	"github.com/streadway/amqp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	pb "go-commerce/api/order"
	"go-commerce/internal/order"
	"go-commerce/internal/outbox"
	"go-commerce/pkg/events"
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

	if err := db.AutoMigrate(&order.Order{}, &order.OrderItem{}, &outbox.Event{}); err != nil {
		log.Fatalf("mysql_migrate_failed error=%v", err)
	}

	paymentTimeout, err := order.ParseOrderPaymentTimeoutMinutes(serviceutil.Env("ORDER_PAYMENT_TIMEOUT_MINUTES", "15"))
	if err != nil {
		log.Fatalf("invalid_order_payment_timeout error=%v", err)
	}

	exchangeName := serviceutil.Env("EVENT_EXCHANGE", mq.DefaultExchangeName)
	rabbitConn, err := amqp.Dial(serviceutil.Env("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"))
	if err != nil {
		log.Printf("rabbitmq_connection_failed exchange=%s error=%v", exchangeName, err)
	}

	var publisher mq.Publisher = mq.NopPublisher{}
	var timeoutScheduler order.TimeoutScheduler = order.NopTimeoutScheduler{}
	var paymentConsumerChannel *amqp.Channel
	var timeoutConsumerChannel *amqp.Channel
	if rabbitConn != nil {
		defer rabbitConn.Close()
		log.Printf("rabbitmq_connected")

		publisherChannel, channelErr := rabbitConn.Channel()
		if channelErr != nil {
			log.Printf("rabbitmq_channel_open_failed exchange=%s error=%v", exchangeName, channelErr)
		} else {
			defer publisherChannel.Close()
			publisher = mq.NewRabbitMQPublisher(publisherChannel, exchangeName)
		}

		paymentConsumerChannel, channelErr = rabbitConn.Channel()
		if channelErr != nil {
			log.Printf("rabbitmq_payment_consumer_channel_open_failed exchange=%s error=%v", exchangeName, channelErr)
		} else {
			defer paymentConsumerChannel.Close()
		}

		timeoutSchedulerChannel, channelErr := rabbitConn.Channel()
		if channelErr != nil {
			log.Printf("rabbitmq_timeout_scheduler_channel_open_failed error=%v", channelErr)
		} else {
			defer timeoutSchedulerChannel.Close()
			if err := declareOrderTimeoutTopology(timeoutSchedulerChannel); err != nil {
				log.Printf("rabbitmq_timeout_topology_declare_failed error=%v", err)
			} else {
				timeoutScheduler = order.NewRabbitMQTimeoutScheduler(timeoutSchedulerChannel, order.OrderTimeoutDelayExchange)
			}
		}

		timeoutConsumerChannel, channelErr = rabbitConn.Channel()
		if channelErr != nil {
			log.Printf("rabbitmq_timeout_consumer_channel_open_failed error=%v", channelErr)
		} else {
			defer timeoutConsumerChannel.Close()
		}
	}

	healthServer := serviceutil.StartHTTPServer(
		"order health server",
		serviceutil.Env("ORDER_HEALTH_ADDR", ":8083"),
		healthcheck.Handler(
			healthcheck.Dependency{Name: "mysql", Check: healthcheck.SQL(sqlDB)},
			healthcheck.Dependency{Name: "rabbitmq", Check: healthcheck.AMQP(rabbitConn)},
		),
	)

	grpcAddr := serviceutil.Env("ORDER_GRPC_ADDR", ":50053")
	listener, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("grpc_listen_failed addr=%s error=%v", grpcAddr, err)
	}
	server := grpc.NewServer()
	pb.RegisterOrderServiceServer(server, order.NewServiceWithTimeout(db, publisher, timeoutScheduler, paymentTimeout))
	grpcHealth := healthcheck.RegisterGRPC(server)

	if paymentConsumerChannel != nil {
		go consumePaymentSucceededEvents(paymentConsumerChannel, exchangeName, db, publisher)
	}
	if timeoutConsumerChannel != nil {
		go consumeOrderTimeoutEvents(timeoutConsumerChannel, db, publisher)
	}

	serveErr := make(chan error, 1)
	go func() {
		log.Printf("order service listening at %s", grpcAddr)
		serveErr <- server.Serve(listener)
	}()

	select {
	case err := <-serveErr:
		if err != nil {
			log.Fatalf("grpc_server_failed error=%v", err)
		}
	case <-ctx.Done():
		log.Printf("shutdown_started signal=%v", ctx.Err())
	}

	grpcHealth.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	serviceutil.ShutdownHTTPServer(healthServer, 5*time.Second)
	serviceutil.ShutdownGRPCServer(server, 10*time.Second)
	log.Printf("order service shutdown completed")
}

func declareOrderTimeoutTopology(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(order.OrderTimeoutDelayExchange, "direct", true, false, false, false, nil); err != nil {
		return err
	}
	if err := ch.ExchangeDeclare(order.OrderTimeoutDLX, "direct", true, false, false, false, nil); err != nil {
		return err
	}
	delayQueue, err := ch.QueueDeclare(
		order.OrderTimeoutDelayQueue,
		true,
		false,
		false,
		false,
		amqp.Table{
			"x-dead-letter-exchange":    order.OrderTimeoutDLX,
			"x-dead-letter-routing-key": events.OrderTimeoutCheckType,
		},
	)
	if err != nil {
		return err
	}
	if err := ch.QueueBind(delayQueue.Name, events.OrderTimeoutCheckType, order.OrderTimeoutDelayExchange, false, nil); err != nil {
		return err
	}

	cancelQueue, err := ch.QueueDeclare(order.OrderTimeoutCancelQueue, true, false, false, false, nil)
	if err != nil {
		return err
	}
	return ch.QueueBind(cancelQueue.Name, events.OrderTimeoutCheckType, order.OrderTimeoutDLX, false, nil)
}

func consumePaymentSucceededEvents(ch *amqp.Channel, exchangeName string, db *gorm.DB, publisher mq.Publisher) {
	if err := ch.ExchangeDeclare(exchangeName, "topic", true, false, false, false, nil); err != nil {
		log.Printf("rabbitmq_exchange_declare_failed exchange=%s error=%v", exchangeName, err)
		return
	}
	queue, err := ch.QueueDeclare("order.payment.succeeded", true, false, false, false, nil)
	if err != nil {
		log.Printf("rabbitmq_queue_declare_failed queue=order.payment.succeeded error=%v", err)
		return
	}
	if err := ch.QueueBind(queue.Name, events.PaymentSucceededType, exchangeName, false, nil); err != nil {
		log.Printf("rabbitmq_queue_bind_failed queue=%s routing_key=%s error=%v", queue.Name, events.PaymentSucceededType, err)
		return
	}
	deliveries, err := ch.Consume(queue.Name, "", false, false, false, false, nil)
	if err != nil {
		log.Printf("rabbitmq_consume_failed queue=%s error=%v", queue.Name, err)
		return
	}

	consumer := order.NewPaymentSucceededConsumer(db, publisher, log.Default())
	log.Printf("order_payment_consumer_started exchange=%s queue=%s routing_key=%s", exchangeName, queue.Name, events.PaymentSucceededType)
	for delivery := range deliveries {
		if err := consumer.HandleDelivery(delivery); err != nil {
			log.Printf("payment_event_handle_failed error=%v", err)
		}
	}
}

func consumeOrderTimeoutEvents(ch *amqp.Channel, db *gorm.DB, publisher mq.Publisher) {
	if err := declareOrderTimeoutTopology(ch); err != nil {
		log.Printf("rabbitmq_timeout_topology_declare_failed error=%v", err)
		return
	}
	deliveries, err := ch.Consume(order.OrderTimeoutCancelQueue, "", false, false, false, false, nil)
	if err != nil {
		log.Printf("rabbitmq_consume_failed queue=%s error=%v", order.OrderTimeoutCancelQueue, err)
		return
	}

	consumer := order.NewOrderTimeoutConsumer(db, publisher, log.Default())
	log.Printf(
		"order_timeout_consumer_started delay_exchange=%s dlx=%s queue=%s routing_key=%s",
		order.OrderTimeoutDelayExchange,
		order.OrderTimeoutDLX,
		order.OrderTimeoutCancelQueue,
		events.OrderTimeoutCheckType,
	)
	for delivery := range deliveries {
		if err := consumer.HandleDelivery(delivery); err != nil {
			log.Printf("order_timeout_event_handle_failed error=%v", err)
		}
	}
}
