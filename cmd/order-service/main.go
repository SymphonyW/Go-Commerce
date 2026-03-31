// order-service 服务入口文件
// 负责启动订单服务，处理订单的创建、查询和取消
// 使用gRPC协议提供服务，并集成RabbitMQ进行消息传递
package main

import (
	"log"
	"net"
	"os"

	// RabbitMQ客户端：用于消息队列操作
	"github.com/streadway/amqp"
	// MySQL驱动：用于连接MySQL数据库
	"gorm.io/driver/mysql"
	// GORM：ORM框架，用于数据库操作
	"gorm.io/gorm"
	// gRPC服务器：用于提供gRPC服务
	"google.golang.org/grpc"

	// 导入订单服务的业务逻辑
	"go-commerce/internal/order"
	// 导入订单服务的protobuf生成代码
	pb "go-commerce/api/order"
)

// main 函数是order-service服务的入口点
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
	// 会根据order.Order和order.OrderItem结构体自动创建或更新数据库表
	if err := db.AutoMigrate(&order.Order{}, &order.OrderItem{}); err != nil {
		log.Fatalf("failed to migrate database: %v", err)
	}

	// 从环境变量获取RabbitMQ连接地址
	rabbitmqURL := os.Getenv("RABBITMQ_URL")
	if rabbitmqURL == "" {
		// 默认值，用于本地开发
		rabbitmqURL = "amqp://guest:guest@localhost:5672/"
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
		log.Fatalf("failed to open a channel: %v", err)
	}
	defer ch.Close()

	// 声明RabbitMQ交换机
	// 创建一个名为"ecommerce"的topic类型交换机
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
	// 监听50053端口，用于gRPC服务
	lis, err := net.Listen("tcp", ":50053")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	
	// 创建gRPC服务器
	s := grpc.NewServer()
	
	// 注册订单服务
	// 将order.NewService(db, ch)创建的服务实例注册到gRPC服务器
	// 传入数据库连接和RabbitMQ通道
	pb.RegisterOrderServiceServer(s, order.NewService(db, ch))
	
	// 启动服务
	// 打印服务监听地址
	log.Printf("order service listening at %v", lis.Addr())
	// 启动gRPC服务器，开始接受请求
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
