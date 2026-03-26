// cart-service 服务入口文件
// 负责启动购物车服务，处理购物车的添加、更新和删除操作
// 使用gRPC协议提供服务，并使用Redis存储购物车数据
package main

import (
	"log"
	"net"

	// Redis客户端：用于存储购物车数据
	"github.com/go-redis/redis/v8"
	// gRPC服务器：用于提供gRPC服务
	"google.golang.org/grpc"
	// gRPC无安全凭据：用于连接其他微服务
	"google.golang.org/grpc/credentials/insecure"

	// 导入购物车服务的protobuf生成代码
	pb "go-commerce/api/cart"
	// 导入产品服务的protobuf生成代码
	pbProduct "go-commerce/api/product"
	// 导入购物车服务的业务逻辑
	"go-commerce/internal/cart"
)

// main 函数是cart-service服务的入口点
// 负责初始化Redis连接、连接产品服务、启动gRPC服务器
func main() {
	// 连接Redis
	// 创建Redis客户端，连接本地Redis服务器
	redisClient := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",  // Redis服务器地址
		Password: "",                 // 无密码
		DB:       0,                  // 使用默认数据库
	})

	// 连接产品服务
	// 用于获取产品信息
	productServiceAddr := "localhost:50052"
	productConn, err := grpc.Dial(productServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect to product service: %v", err)
	}
	defer productConn.Close()
	productClient := pbProduct.NewProductServiceClient(productConn)

	// 监听TCP端口
	// 监听50054端口，用于gRPC服务
	lis, err := net.Listen("tcp", ":50054")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	
	// 创建gRPC服务器
	s := grpc.NewServer()
	
	// 注册购物车服务
	// 将cart.NewService(redisClient, productClient)创建的服务实例注册到gRPC服务器
	// 传入Redis客户端和产品服务客户端
	pb.RegisterCartServiceServer(s, cart.NewService(redisClient, productClient))
	
	// 启动服务
	// 打印服务监听地址
	log.Printf("cart service listening at %v", lis.Addr())
	// 启动gRPC服务器，开始接受请求
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
