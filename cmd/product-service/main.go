// product-service 服务入口文件
// 负责启动产品服务，处理商品的查询和管理
// 使用gRPC协议提供服务
package main

import (
	"log"
	"net"

	// gRPC服务器：用于提供gRPC服务
	"google.golang.org/grpc"
	// MySQL驱动：用于连接MySQL数据库
	"gorm.io/driver/mysql"
	// GORM：ORM框架，用于数据库操作
	"gorm.io/gorm"

	// 导入产品服务的protobuf生成代码
	pb "go-commerce/api/product"
	// 导入产品服务的业务逻辑
	"go-commerce/internal/product"
)

// main 函数是product-service服务的入口点
// 负责初始化数据库连接、自动迁移表结构、启动gRPC服务器
func main() {
	// 数据库连接字符串
	// 格式：用户名:密码@tcp(主机:端口)/数据库名?参数
	dsn := "root:password@tcp(127.0.0.1:3307)/ecommerce?charset=utf8mb4&parseTime=True&loc=Local"

	// 连接数据库
	// 使用GORM打开数据库连接
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	// 自动迁移数据库表结构
	// 会根据product.Product结构体自动创建或更新数据库表
	if err := db.AutoMigrate(&product.Product{}); err != nil {
		log.Fatalf("failed to migrate database: %v", err)
	}

	// 监听TCP端口
	// 监听50052端口，用于gRPC服务
	lis, err := net.Listen("tcp", ":50052")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	// 创建gRPC服务器
	s := grpc.NewServer()

	// 注册产品服务
	// 将product.NewService(db)创建的服务实例注册到gRPC服务器
	pb.RegisterProductServiceServer(s, product.NewService(db))

	// 启动服务
	// 打印服务监听地址
	log.Printf("product service listening at %v", lis.Addr())
	// 启动gRPC服务器，开始接受请求
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
