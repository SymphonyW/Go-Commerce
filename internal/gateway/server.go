package gateway

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"go-commerce/internal/gateway/handler"
	"go-commerce/pkg/healthcheck"
	"go-commerce/pkg/observability"
)

type ServerConfig struct {
	HTTPAddr           string
	Clients            handler.Clients
	Logger             *slog.Logger
	Registry           *prometheus.Registry
	Metrics            *observability.Metrics
	CORSAllowedOrigins []string
	HealthDependencies []healthcheck.Dependency
}

type Server struct {
	httpServer *http.Server
}

func NewServer(config ServerConfig) *Server {
	addr := config.HTTPAddr
	if addr == "" {
		addr = ":8080"
	}

	router := NewRouter(RouterConfig{
		Clients:            config.Clients,
		Logger:             config.Logger,
		Registry:           config.Registry,
		Metrics:            config.Metrics,
		CORSAllowedOrigins: config.CORSAllowedOrigins,
		HealthDependencies: config.HealthDependencies,
	})
	return &Server{
		httpServer: &http.Server{
			Addr:              addr,
			Handler:           router,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}
}

func (s *Server) Addr() string {
	if s == nil || s.httpServer == nil {
		return ""
	}
	return s.httpServer.Addr
}

func (s *Server) ListenAndServe() error {
	if s == nil || s.httpServer == nil {
		return http.ErrServerClosed
	}
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s == nil || s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}
