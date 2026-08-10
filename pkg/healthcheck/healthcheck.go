package healthcheck

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/streadway/amqp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// Checker 负责描述一个 readiness 依赖是否可用。
type Checker func(context.Context) error

// Dependency 将依赖名称和其检查函数绑定在一起，便于 readyz 返回可读错误。
type Dependency struct {
	Name  string
	Check Checker
}

// Handler 暴露 /healthz 与 /readyz，前者只看进程，后者检查关键依赖。
func Handler(dependencies ...Dependency) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		failures := make(map[string]string)
		dependencyStatus := make(map[string]string)
		for _, dependency := range dependencies {
			if dependency.Check == nil {
				continue
			}
			if err := dependency.Check(ctx); err != nil {
				failures[dependency.Name] = err.Error()
				continue
			}
			dependencyStatus[dependency.Name] = "ok"
		}
		if len(failures) > 0 {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"status":       "not_ready",
				"dependencies": failures,
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status":       "ready",
			"dependencies": dependencyStatus,
		})
	})
	return mux
}

func HandlerWithMetrics(metrics http.Handler, dependencies ...Dependency) http.Handler {
	probe := Handler(dependencies...)
	mux := http.NewServeMux()
	mux.Handle("/healthz", probe)
	mux.Handle("/readyz", probe)
	if metrics != nil {
		mux.Handle("/metrics", metrics)
	}
	return mux
}

// SQL 使用 PingContext 验证数据库连接池是否仍然可用。
func SQL(db *sql.DB) Checker {
	return func(ctx context.Context) error {
		if db == nil {
			return fmt.Errorf("database is not initialized")
		}
		return db.PingContext(ctx)
	}
}

// Redis 使用 PING 验证缓存依赖是否可用。
func Redis(client *redis.Client) Checker {
	return func(ctx context.Context) error {
		if client == nil {
			return fmt.Errorf("redis is not initialized")
		}
		return client.Ping(ctx).Err()
	}
}

// AMQP 至少确认连接未被关闭，避免把消息链路故障误报成 ready。
func AMQP(conn *amqp.Connection) Checker {
	return func(_ context.Context) error {
		if conn == nil {
			return fmt.Errorf("rabbitmq is not connected")
		}
		if conn.IsClosed() {
			return fmt.Errorf("rabbitmq connection is closed")
		}
		return nil
	}
}

// GRPCHealth 通过标准 gRPC health 协议验证关键下游已经真正可服务。
func GRPCHealth(conn *grpc.ClientConn, serviceName string) Checker {
	return func(ctx context.Context) error {
		if conn == nil {
			return fmt.Errorf("grpc connection is not initialized")
		}
		resp, err := grpc_health_v1.NewHealthClient(conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{
			Service: serviceName,
		})
		if err != nil {
			return err
		}
		if resp.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
			return fmt.Errorf("grpc health status is %s", resp.GetStatus().String())
		}
		return nil
	}
}

// RegisterGRPC 为 gRPC 服务注册标准 health service，并默认标记为 SERVING。
func RegisterGRPC(server *grpc.Server) *health.Server {
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(server, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	return healthServer
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

// OrderedFailureNames 主要服务于测试，确保失败输出在断言时具备稳定顺序。
func OrderedFailureNames(failures map[string]string) []string {
	names := make([]string, 0, len(failures))
	for name := range failures {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
