// 订单服务入口文件
// 负责启动订单服务，处理订单的创建、查询和取消
package main

import (
	"log"
	"net"

	"github.com/streadway/amqp"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"google.golang.org/grpc"

	"go-commerce/internal/order"
	pb "go-commerce/api/order"
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
	if err := db.AutoMigrate(&order.Order{}, &order.OrderItem{}); err != nil {
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
		log.Fatalf("failed to open a channel: %v", err)
	}
	defer ch.Close()

	// 声明RabbitMQ交换机
	err = ch.ExchangeDeclare(
		"ecommerce", // 交换机名称
		"topic",     // 交换机类型
		true,        // 持久化
		false,       // 自动删除
		false,       // 内部
		false,       // 不等待
		nil,         // 参数
	)
	if err != nil {
		log.Fatalf("failed to declare an exchange: %v", err)
	}

	// 监听TCP端口
	lis, err := net.Listen("tcp", ":50053")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	
	// 创建gRPC服务器
	s := grpc.NewServer()
	
	// 注册订单服务
	pb.RegisterOrderServiceServer(s, order.NewService(db, ch))
	
	// 启动服务
	log.Printf("order service listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
