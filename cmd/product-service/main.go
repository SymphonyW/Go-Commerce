package main

import (
	"context"
	"log"
	"net"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	pb "go-commerce/api/product"
	"go-commerce/internal/product"
	"go-commerce/pkg/healthcheck"
	"go-commerce/pkg/observability"
	"go-commerce/pkg/serviceutil"
)

func main() {
	ctx, stop := serviceutil.SignalContext()
	defer stop()
	telemetry := observability.SetupService(ctx, "product-service")
	logger := telemetry.Logger
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := telemetry.Shutdown(shutdownCtx); err != nil {
			logger.Error("otel_shutdown_failed", "error", err)
		}
	}()

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

	if serviceutil.AutoMigrateEnabled() {
		log.Printf("auto_migrate_enabled warning=use_cmd_migrate_for_shared_mysql")
		if err := db.AutoMigrate(&product.Product{}); err != nil {
			log.Fatalf("mysql_migrate_failed error=%v", err)
		}
	} else {
		log.Printf("auto_migrate_disabled command=\"go run ./cmd/migrate up\"")
	}

	healthServer := serviceutil.StartHTTPServer(
		"product health server",
		serviceutil.Env("PRODUCT_HEALTH_ADDR", ":8082"),
		healthcheck.HandlerWithMetrics(
			promhttp.HandlerFor(telemetry.Registry, promhttp.HandlerOpts{}),
			healthcheck.Dependency{Name: "mysql", Check: healthcheck.SQL(sqlDB)},
		),
	)

	grpcAddr := serviceutil.Env("PRODUCT_GRPC_ADDR", ":50052")
	listener, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("grpc_listen_failed addr=%s error=%v", grpcAddr, err)
	}
	server := grpc.NewServer(grpc.UnaryInterceptor(observability.UnaryServerInterceptor(logger, telemetry.Metrics)))
	pb.RegisterProductServiceServer(server, product.NewService(db))
	grpcHealth := healthcheck.RegisterGRPC(server)

	serveErr := make(chan error, 1)
	go func() {
		log.Printf("product service listening at %s", grpcAddr)
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
	log.Printf("product service shutdown completed")
}
