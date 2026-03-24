package main

import (
	"fmt"
	"log"
	"net"

	"github.com/streadway/amqp"
	"google.golang.org/grpc"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	pb "go-commerce/api/merchant"
	"go-commerce/internal/merchant"
)

func main() {
	// 连接数据库
	dsn := "root:password@tcp(127.0.0.1:3307)/ecommerce?charset=utf8mb4&parseTime=True&loc=Local"
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	// 自动迁移数据库表
	if err := db.AutoMigrate(&merchant.Merchant{}); err != nil {
		log.Fatalf("failed to migrate database: %v", err)
	}

	// 连接RabbitMQ
	conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	if err != nil {
		log.Fatalf("failed to connect to RabbitMQ: %v", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("failed to open RabbitMQ channel: %v", err)
	}
	defer ch.Close()

	// 创建商家服务实例
	merchantService := merchant.NewGRPCService(db)

	// 启动gRPC服务器
	listener, err := net.Listen("tcp", ":50055")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	// 注册服务
	server := grpc.NewServer()
	pb.RegisterMerchantServiceServer(server, merchantService)

	fmt.Println("Merchant service is running on port 50055")
	if err := server.Serve(listener); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
