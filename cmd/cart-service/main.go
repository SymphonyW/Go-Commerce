package main

import (
	"context"
	"log"
	"net"
	"time"

	"github.com/go-redis/redis/v8"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"

	pb "go-commerce/api/cart"
	pbProduct "go-commerce/api/product"
	"go-commerce/internal/cart"
	"go-commerce/pkg/healthcheck"
	"go-commerce/pkg/observability"
	"go-commerce/pkg/serviceutil"
)

func main() {
	ctx, stop := serviceutil.SignalContext()
	defer stop()

	redisClient := redis.NewClient(&redis.Options{
		Addr: serviceutil.Env("REDIS_ADDR", "127.0.0.1:6379"),
		DB:   0,
	})
	defer redisClient.Close()
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("redis_connect_failed error=%v", err)
	}
	log.Printf("redis_connected")

	grpcTimeout := serviceutil.DurationEnv("SERVICE_GRPC_TIMEOUT", 3*time.Second)
	productConn, err := grpc.Dial(
		serviceutil.Env("PRODUCT_SERVICE_ADDR", "localhost:50052"),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(observability.UnaryClientTimeoutInterceptor(grpcTimeout)),
	)
	if err != nil {
		log.Fatalf("product_service_dial_failed error=%v", err)
	}
	defer productConn.Close()
	productClient := pbProduct.NewProductServiceClient(productConn)

	healthServer := serviceutil.StartHTTPServer(
		"cart health server",
		serviceutil.Env("CART_HEALTH_ADDR", ":8084"),
		healthcheck.Handler(
			healthcheck.Dependency{Name: "redis", Check: healthcheck.Redis(redisClient)},
			healthcheck.Dependency{Name: "product-service", Check: healthcheck.GRPCHealth(productConn, "")},
		),
	)

	grpcAddr := serviceutil.Env("CART_GRPC_ADDR", ":50054")
	listener, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("grpc_listen_failed addr=%s error=%v", grpcAddr, err)
	}
	server := grpc.NewServer()
	pb.RegisterCartServiceServer(server, cart.NewService(redisClient, productClient))
	grpcHealth := healthcheck.RegisterGRPC(server)

	serveErr := make(chan error, 1)
	go func() {
		log.Printf("cart service listening at %s", grpcAddr)
		serveErr <- server.Serve(listener)
	}()

	select {
	case err := <-serveErr:
		if err != nil {
			log.Fatalf("grpc_server_failed error=%v", err)
		}
	case <-ctx.Done():
		log.Printf("shutdown_started signal=%v", ctx.Err())
	}

	grpcHealth.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	serviceutil.ShutdownHTTPServer(healthServer, 5*time.Second)
	serviceutil.ShutdownGRPCServer(server, 10*time.Second)
	log.Printf("cart service shutdown completed")
}
