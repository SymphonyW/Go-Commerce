// auth-service 服务入口文件
// 负责启动认证服务，处理用户注册、登录等认证相关功能
// 使用gRPC协议提供服务
package main

import (
	"log"
	"net"
	"os"

	// gRPC服务器：用于提供gRPC服务
	"google.golang.org/grpc"
	// MySQL驱动：用于连接MySQL数据库
	"gorm.io/driver/mysql"
	// GORM：ORM框架，用于数据库操作
	"gorm.io/gorm"

	// 导入认证服务的protobuf生成代码
	pb "go-commerce/api/auth"
	// 导入认证服务的业务逻辑
	"go-commerce/internal/auth"
)

// main 函数是auth-service服务的入口点
// 负责初始化数据库连接、自动迁移表结构、启动gRPC服务器
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
	// 会根据auth.User结构体自动创建或更新数据库表
	if err := db.AutoMigrate(&auth.User{}); err != nil {
		log.Fatalf("failed to migrate database: %v", err)
	}

	// 监听TCP端口
	// 监听50051端口，用于gRPC服务
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	// 创建gRPC服务器
	s := grpc.NewServer()

	// 注册认证服务
	// 将auth.NewService(db)创建的服务实例注册到gRPC服务器
	pb.RegisterAuthServiceServer(s, auth.NewService(db))

	// 启动服务
	// 打印服务监听地址
	log.Printf("auth service listening at %v", lis.Addr())
	// 启动gRPC服务器，开始接受请求
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
