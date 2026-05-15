package main

import (
	"log"
	"net"
	"os"

	"github.com/streadway/amqp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	pbOrder "go-commerce/api/order"
	pbPayment "go-commerce/api/payment"
	"go-commerce/internal/payment"
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
	if err := db.AutoMigrate(&payment.Payment{}); err != nil {
		log.Fatalf("failed to migrate database: %v", err)
	}

	orderServiceAddr := os.Getenv("ORDER_SERVICE_ADDR")
	if orderServiceAddr == "" {
		orderServiceAddr = "localhost:50053"
	}
	orderConn, err := grpc.Dial(orderServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect to order service: %v", err)
	}
	defer orderConn.Close()
	orderClient := pbOrder.NewOrderServiceClient(orderConn)

	rabbitmqURL := os.Getenv("RABBITMQ_URL")
	if rabbitmqURL == "" {
		rabbitmqURL = "amqp://guest:guest@localhost:5672/"
	}
	exchangeName := os.Getenv("EVENT_EXCHANGE")
	if exchangeName == "" {
		exchangeName = mq.DefaultExchangeName
	}

	var publisher mq.Publisher = mq.NopPublisher{}
	conn, err := amqp.Dial(rabbitmqURL)
	if err != nil {
		log.Printf("rabbitmq_connection_failed exchange=%s error=%v", exchangeName, err)
	} else {
		defer conn.Close()
		ch, channelErr := conn.Channel()
		if channelErr != nil {
			log.Printf("rabbitmq_channel_open_failed exchange=%s error=%v", exchangeName, channelErr)
		} else {
			defer ch.Close()
			publisher = mq.NewRabbitMQPublisher(ch, exchangeName)
		}
	}

	lis, err := net.Listen("tcp", ":50056")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	server := grpc.NewServer()
	core := payment.NewService(db, orderClient, publisher)
	pbPayment.RegisterPaymentServiceServer(server, payment.NewGRPCService(core))

	log.Printf("payment service listening at %v", lis.Addr())
	if err := server.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
