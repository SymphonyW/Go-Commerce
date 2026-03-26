// 产品服务入口文件
// 负责启动产品服务，处理商品的查询和管理
package main

import (
	"log"
	"net"

	"google.golang.org/grpc"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	pb "go-commerce/api/product"
	"go-commerce/internal/product"
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
	if err := db.AutoMigrate(&product.Product{}); err != nil {
		log.Fatalf("failed to migrate database: %v", err)
	}

	// 监听TCP端口
	lis, err := net.Listen("tcp", ":50052")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	// 创建gRPC服务器
	s := grpc.NewServer()

	// 注册产品服务
	pb.RegisterProductServiceServer(s, product.NewService(db))

	// 启动服务
	log.Printf("product service listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
