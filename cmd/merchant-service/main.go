// 商户服务入口文件
// 负责启动商户服务，处理商户的创建和管理
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
	// 数据库连接字符串
	dsn := "root:password@tcp(127.0.0.1:3307)/ecommerce?charset=utf8mb4&parseTime=True&loc=Local"
	
	// 连接数据库
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	// 自动迁移数据库表结构
	if err := db.AutoMigrate(&merchant.Merchant{}); err != nil {
		log.Fatalf("failed to migrate database: %v", err)
	}

	// 连接RabbitMQ
	conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	if err != nil {
		log.Fatalf("failed to connect to RabbitMQ: %v", err)
	}
	defer conn.Close()

	// 创建RabbitMQ通道
	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("failed to open RabbitMQ channel: %v", err)
	}
	defer ch.Close()

	// 创建商家服务实例
	merchantService := merchant.NewGRPCService(db)

	// 监听TCP端口
	listener, err := net.Listen("tcp", ":50055")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	// 创建gRPC服务器
	server := grpc.NewServer()
	
	// 注册商户服务
	pb.RegisterMerchantServiceServer(server, merchantService)

	// 启动服务
	fmt.Println("Merchant service is running on port 50055")
	if err := server.Serve(listener); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
