// 购物车服务入口文件
// 负责启动购物车服务，处理购物车的添加、更新和删除操作
package main

import (
	"log"
	"net"

	"github.com/go-redis/redis/v8"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "go-commerce/api/cart"
	pbProduct "go-commerce/api/product"
	"go-commerce/internal/cart"
)

func main() {
	// 连接Redis
	redisClient := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // 无密码
		DB:       0,  // 使用默认数据库
	})

	// 连接产品服务
	productServiceAddr := "localhost:50052"
	productConn, err := grpc.Dial(productServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect to product service: %v", err)
	}
	defer productConn.Close()
	productClient := pbProduct.NewProductServiceClient(productConn)

	// 监听TCP端口
	lis, err := net.Listen("tcp", ":50054")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	
	// 创建gRPC服务器
	s := grpc.NewServer()
	
	// 注册购物车服务
	pb.RegisterCartServiceServer(s, cart.NewService(redisClient, productClient))
	
	// 启动服务
	log.Printf("cart service listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
