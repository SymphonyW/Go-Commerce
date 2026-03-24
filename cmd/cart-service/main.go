package main

import (
	"log"
	"net"

	"github.com/go-redis/redis/v8"
	"google.golang.org/grpc"

	pb "go-commerce/api/cart"
	"go-commerce/internal/cart"
)

func main() {
	redisClient := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	lis, err := net.Listen("tcp", ":50054")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterCartServiceServer(s, cart.NewService(redisClient))
	log.Printf("cart service listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
