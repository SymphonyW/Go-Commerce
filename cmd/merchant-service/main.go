// merchant-service 服务入口文件
// 负责启动商户服务，处理商户的创建和管理
// 使用gRPC协议提供服务，并集成RabbitMQ进行消息传递
package main

import (
	"fmt"
	"log"
	"net"
	"os"

	// RabbitMQ客户端：用于消息队列操作
	"github.com/streadway/amqp"
	// gRPC服务器：用于提供gRPC服务
	"google.golang.org/grpc"
	// MySQL驱动：用于连接MySQL数据库
	"gorm.io/driver/mysql"
	// GORM：ORM框架，用于数据库操作
	"gorm.io/gorm"

	// 导入商户服务的protobuf生成代码
	pb "go-commerce/api/merchant"
	// 导入商户服务的业务逻辑
	"go-commerce/internal/merchant"
)

// main 函数是merchant-service服务的入口点
// 负责初始化数据库连接、自动迁移表结构、连接RabbitMQ、启动gRPC服务器
func main() {
	// 从环境变量获取数据库连接字符串
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		// 默认值，用于本地开发
		dsn = "root:password@tcp(127.0.0.1:3307)/ecommerce?charset=utf8mb4&parseTime=True&loc=Local"
	}
	
	// 连接数据库
	// 使用GORM打开数据库连接
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	// 自动迁移数据库表结构
	// 会根据merchant.Merchant结构体自动创建或更新数据库表
	if err := db.AutoMigrate(&merchant.Merchant{}); err != nil {
		log.Fatalf("failed to migrate database: %v", err)
	}

	// 从环境变量获取RabbitMQ连接地址
	rabbitmqURL := os.Getenv("RABBITMQ_URL")
	if rabbitmqURL == "" {
		// 默认值，用于本地开发
		rabbitmqURL = "amqp://guest:guest@rabbitmq:5672/"
	}

	// 连接RabbitMQ
	// 使用默认的guest账号连接本地RabbitMQ服务器
	conn, err := amqp.Dial(rabbitmqURL)
	if err != nil {
		log.Fatalf("failed to connect to RabbitMQ: %v", err)
	}
	defer conn.Close()

	// 创建RabbitMQ通道
	// 用于执行RabbitMQ操作
	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("failed to open RabbitMQ channel: %v", err)
	}
	defer ch.Close()

	// 创建商家服务实例
	// 传入数据库连接
	merchantService := merchant.NewGRPCService(db)

	// 监听TCP端口
	// 监听50055端口，用于gRPC服务
	listener, err := net.Listen("tcp", ":50055")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	// 创建gRPC服务器
	server := grpc.NewServer()
	
	// 注册商户服务
	// 将merchantService注册到gRPC服务器
	pb.RegisterMerchantServiceServer(server, merchantService)

	// 启动服务
	// 打印服务运行信息
	fmt.Println("Merchant service is running on port 50055")
	// 启动gRPC服务器，开始接受请求
	if err := server.Serve(listener); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
