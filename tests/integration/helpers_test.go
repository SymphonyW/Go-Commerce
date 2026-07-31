//go:build integration

package integration_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	orderdomain "go-commerce/internal/order"

	"github.com/go-redis/redis/v8"
	gomysql "github.com/go-sql-driver/mysql"
	"github.com/streadway/amqp"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func integrationMySQLDSN() string {
	if value := os.Getenv("INTEGRATION_DB_DSN"); value != "" {
		return value
	}
	return "root:password@tcp(127.0.0.1:3307)/ecommerce?charset=utf8mb4&parseTime=True&loc=Local"
}

func integrationRedisAddr() string {
	if value := os.Getenv("INTEGRATION_REDIS_ADDR"); value != "" {
		return value
	}
	return "127.0.0.1:6379"
}

func integrationRabbitMQURL() string {
	if value := os.Getenv("INTEGRATION_RABBITMQ_URL"); value != "" {
		return value
	}
	return "amqp://guest:guest@127.0.0.1:5672/"
}

func openIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()

	config, err := gomysql.ParseDSN(integrationMySQLDSN())
	if err != nil {
		t.Fatalf("failed to parse integration mysql dsn: %v", err)
	}

	// 每次测试创建独立 schema，避免与开发库或其他测试共享表状态。
	databaseName := fmt.Sprintf("ecommerce_test_%s", strings.ReplaceAll(uniqueSuffix(t), "-", "_"))
	adminConfig := config.Clone()
	adminConfig.DBName = ""

	adminDB, err := sql.Open("mysql", adminConfig.FormatDSN())
	if err != nil {
		t.Fatalf("failed to open integration mysql admin connection: %v", err)
	}
	t.Cleanup(func() { _ = adminDB.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := adminDB.PingContext(ctx); err != nil {
		t.Fatalf("failed to ping integration mysql admin connection: %v", err)
	}
	if _, err := adminDB.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE `%s`", databaseName)); err != nil {
		t.Fatalf("failed to create integration mysql database: %v", err)
	}
	t.Cleanup(func() {
		_, _ = adminDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", databaseName))
	})

	config.DBName = databaseName
	db, err := gorm.Open(mysql.Open(config.FormatDSN()), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open integration mysql: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get integration sql db: %v", err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		t.Fatalf("failed to ping integration mysql: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func ensureIntegrationOrderIndexes(t *testing.T, db *gorm.DB) {
	t.Helper()

	if err := orderdomain.EnsureOrderIndexes(db); err != nil {
		t.Fatalf("failed to migrate integration order indexes: %v", err)
	}
}

func openIntegrationRedis(t *testing.T) *redis.Client {
	t.Helper()

	client := redis.NewClient(&redis.Options{Addr: integrationRedisAddr()})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("failed to ping integration redis: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func openIntegrationRabbitMQ(t *testing.T) *amqp.Connection {
	t.Helper()

	conn, err := amqp.Dial(integrationRabbitMQURL())
	if err != nil {
		t.Fatalf("failed to connect integration rabbitmq: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func uniqueSuffix(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("%d", time.Now().UTC().UnixNano())
}
