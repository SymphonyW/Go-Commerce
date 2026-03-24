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
	redisClient := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	// 连接产品服务
	productServiceAddr := "localhost:50052"
	productConn, err := grpc.Dial(productServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect to product service: %v", err)
	}
	defer productConn.Close()
	productClient := pbProduct.NewProductServiceClient(productConn)

	lis, err := net.Listen("tcp", ":50054")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterCartServiceServer(s, cart.NewService(redisClient, productClient))
	log.Printf("cart service listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
