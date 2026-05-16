package main

import (
	"log"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	pb "go-commerce/api/auth"
	"go-commerce/internal/auth"
	"go-commerce/pkg/healthcheck"
	"go-commerce/pkg/serviceutil"
)

func main() {
	ctx, stop := serviceutil.SignalContext()
	defer stop()

	dsn := serviceutil.Env("DB_DSN", "root:password@tcp(127.0.0.1:3307)/ecommerce?charset=utf8mb4&parseTime=True&loc=Local")
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("mysql_connect_failed error=%v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("mysql_handle_failed error=%v", err)
	}
	defer sqlDB.Close()
	log.Printf("mysql_connected")

	if err := db.AutoMigrate(&auth.User{}); err != nil {
		log.Fatalf("mysql_migrate_failed error=%v", err)
	}

	healthServer := serviceutil.StartHTTPServer(
		"auth health server",
		serviceutil.Env("AUTH_HEALTH_ADDR", ":8081"),
		healthcheck.Handler(healthcheck.Dependency{Name: "mysql", Check: healthcheck.SQL(sqlDB)}),
	)

	grpcAddr := serviceutil.Env("AUTH_GRPC_ADDR", ":50051")
	listener, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("grpc_listen_failed addr=%s error=%v", grpcAddr, err)
	}
	server := grpc.NewServer()
	pb.RegisterAuthServiceServer(server, auth.NewService(db))
	grpcHealth := healthcheck.RegisterGRPC(server)

	serveErr := make(chan error, 1)
	go func() {
		log.Printf("auth service listening at %s", grpcAddr)
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
	log.Printf("auth service shutdown completed")
}
