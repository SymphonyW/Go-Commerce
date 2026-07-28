package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"go-commerce/internal/gateway"
	"go-commerce/internal/gateway/middleware"
	"go-commerce/pkg/observability"
	"go-commerce/pkg/serviceutil"
)

func main() {
	ctx, stop := serviceutil.SignalContext()
	defer stop()

	logger := observability.NewLogger("api-gateway")
	slog.SetDefault(logger)
	registry := prometheus.NewRegistry()
	metrics := observability.NewMetrics("api-gateway", registry)

	grpcTimeout := serviceutil.DurationEnv("GATEWAY_GRPC_TIMEOUT", 3*time.Second)
	clients, conns, err := gateway.DialClients(
		gateway.ClientAddresses{
			Auth:     serviceutil.Env("AUTH_SERVICE_ADDR", "localhost:50051"),
			Product:  serviceutil.Env("PRODUCT_SERVICE_ADDR", "localhost:50052"),
			Order:    serviceutil.Env("ORDER_SERVICE_ADDR", "localhost:50053"),
			Cart:     serviceutil.Env("CART_SERVICE_ADDR", "localhost:50054"),
			Merchant: serviceutil.Env("MERCHANT_SERVICE_ADDR", "localhost:50055"),
			Payment:  serviceutil.Env("PAYMENT_SERVICE_ADDR", "localhost:50056"),
		},
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(
			observability.UnaryClientInterceptor(),
			observability.UnaryClientTimeoutInterceptor(grpcTimeout),
		),
	)
	if err != nil {
		logger.Error("grpc_dial_failed", "error", err)
		os.Exit(1)
	}
	defer conns.Close()

	server := gateway.NewServer(gateway.ServerConfig{
		HTTPAddr:           serviceutil.Env("HTTP_ADDR", ":8080"),
		Clients:            clients,
		Logger:             logger,
		Registry:           registry,
		Metrics:            metrics,
		CORSAllowedOrigins: middleware.ParseAllowedOrigins(serviceutil.Env("CORS_ALLOWED_ORIGINS", "")),
		HealthDependencies: conns.HealthDependencies(),
	})

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("http_server_listening", "addr", server.Addr())
		serveErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if err != nil && err != http.ErrServerClosed {
			logger.Error("http_server_failed", "addr", server.Addr(), "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		logger.Info("shutdown_started", "signal", ctx.Err())
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("http_server_shutdown_failed", "addr", server.Addr(), "error", err)
		os.Exit(1)
	}
	logger.Info("api_gateway_shutdown_completed")
}
