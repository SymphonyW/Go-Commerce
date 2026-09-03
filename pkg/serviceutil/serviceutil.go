package serviceutil

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc"
)

// Env 返回环境变量值；未配置时使用统一默认值。
func Env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func BoolEnv(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	switch strings.ToLower(value) {
	case "1", "true", "t", "yes", "y", "on":
		return true
	case "0", "false", "f", "no", "n", "off":
		return false
	default:
		log.Printf("invalid_bool_env key=%s value=%s fallback=%t", key, value, fallback)
		return fallback
	}
}

func AutoMigrateEnabled() bool {
	return BoolEnv("AUTO_MIGRATE_ENABLED", false)
}

// DurationEnv 解析 duration 环境变量，非法值时回退并写明日志。
func DurationEnv(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		log.Printf("invalid_duration_env key=%s value=%s fallback=%s error=%v", key, value, fallback, err)
		return fallback
	}
	return parsed
}

// SignalContext 统一捕获本地 Ctrl+C 与容器停止信号。
func SignalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

// StartHTTPServer 在后台启动探针服务，并把监听地址写入日志。
func StartHTTPServer(name, addr string, handler http.Handler) *http.Server {
	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		log.Printf("%s listening at %s", name, addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("%s failed error=%v", name, err)
		}
	}()
	return server
}

// ShutdownHTTPServer 给探针或业务 HTTP 服务一个有界的优雅退出窗口。
func ShutdownHTTPServer(server *http.Server, timeout time.Duration) {
	if server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("http_server_shutdown_failed addr=%s error=%v", server.Addr, err)
		return
	}
	log.Printf("http_server_shutdown_completed addr=%s", server.Addr)
}

// ShutdownGRPCServer 优先尝试平滑停止，超时后再强制结束，避免容器退出被无限拖住。
func ShutdownGRPCServer(server *grpc.Server, timeout time.Duration) {
	if server == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		server.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("grpc_server_shutdown_completed")
	case <-time.After(timeout):
		log.Printf("grpc_server_graceful_shutdown_timeout timeout=%s", timeout)
		server.Stop()
	}
}
