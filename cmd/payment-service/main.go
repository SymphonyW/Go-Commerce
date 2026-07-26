package main

import (
	"log"
	"net"
	"time"

	"github.com/streadway/amqp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	pbOrder "go-commerce/api/order"
	pbPayment "go-commerce/api/payment"
	"go-commerce/internal/outbox"
	"go-commerce/internal/payment"
	"go-commerce/pkg/healthcheck"
	"go-commerce/pkg/mq"
	"go-commerce/pkg/observability"
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

	if err := payment.Migrate(db); err != nil {
		log.Fatalf("mysql_payment_migrate_failed error=%v", err)
	}
	if err := db.AutoMigrate(&outbox.Event{}); err != nil {
		log.Fatalf("mysql_migrate_failed error=%v", err)
	}

	grpcTimeout := serviceutil.DurationEnv("SERVICE_GRPC_TIMEOUT", 3*time.Second)
	orderConn, err := grpc.Dial(
		serviceutil.Env("ORDER_SERVICE_ADDR", "localhost:50053"),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(observability.UnaryClientTimeoutInterceptor(grpcTimeout)),
	)
	if err != nil {
		log.Fatalf("order_service_dial_failed error=%v", err)
	}
	defer orderConn.Close()
	orderClient := pbOrder.NewOrderServiceClient(orderConn)

	exchangeName := serviceutil.Env("EVENT_EXCHANGE", mq.DefaultExchangeName)
	rabbitConn, err := amqp.Dial(serviceutil.Env("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"))
	if err != nil {
		log.Printf("rabbitmq_connection_failed exchange=%s error=%v", exchangeName, err)
	}
	var publisher mq.Publisher = mq.NopPublisher{}
	var rabbitChannel *amqp.Channel
	if rabbitConn != nil {
		defer rabbitConn.Close()
		rabbitChannel, err = rabbitConn.Channel()
		if err != nil {
			log.Printf("rabbitmq_channel_open_failed exchange=%s error=%v", exchangeName, err)
		} else {
			defer rabbitChannel.Close()
			publisher = mq.NewRabbitMQPublisher(rabbitChannel, exchangeName)
			log.Printf("rabbitmq_connected")
		}
	}

	healthServer := serviceutil.StartHTTPServer(
		"payment health server",
		serviceutil.Env("PAYMENT_HEALTH_ADDR", ":8086"),
		healthcheck.Handler(
			healthcheck.Dependency{Name: "mysql", Check: healthcheck.SQL(sqlDB)},
			healthcheck.Dependency{Name: "order-service", Check: healthcheck.GRPCHealth(orderConn, "")},
			healthcheck.Dependency{Name: "rabbitmq", Check: healthcheck.AMQP(rabbitConn)},
		),
	)

	grpcAddr := serviceutil.Env("PAYMENT_GRPC_ADDR", ":50056")
	listener, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("grpc_listen_failed addr=%s error=%v", grpcAddr, err)
	}
	server := grpc.NewServer()
	core := payment.NewService(db, orderClient, publisher)
	pbPayment.RegisterPaymentServiceServer(server, payment.NewGRPCService(core))
	grpcHealth := healthcheck.RegisterGRPC(server)

	serveErr := make(chan error, 1)
	go func() {
		log.Printf("payment service listening at %s", grpcAddr)
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
	log.Printf("payment service shutdown completed")
}
