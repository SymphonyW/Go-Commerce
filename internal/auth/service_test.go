package auth

import (
	"context"
	"fmt"
	"strings"
	"testing"

	pb "go-commerce/api/auth"
	"go-commerce/pkg/jwt"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newAuthTestService(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()

	dsn := fmt.Sprintf(
		"file:%s?mode=memory&cache=shared",
		strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()),
	)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	if err := db.AutoMigrate(&User{}); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	return NewService(db), db
}

func TestRegisterDefaultsToCustomerRole(t *testing.T) {
	service, db := newAuthTestService(t)

	resp, err := service.Register(context.Background(), &pb.RegisterRequest{
		Username: "customer-user",
		Password: "password123",
		Email:    "customer@example.com",
	})
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if resp.Role != "customer" {
		t.Fatalf("unexpected response role: got %q want %q", resp.Role, "customer")
	}

	var user User
	if err := db.First(&user, resp.UserId).Error; err != nil {
		t.Fatalf("failed to query user: %v", err)
	}
	if user.Role != "customer" {
		t.Fatalf("unexpected persisted role: got %q want %q", user.Role, "customer")
	}

	claims, err := jwt.ValidateToken(resp.Token)
	if err != nil {
		t.Fatalf("ValidateToken returned error: %v", err)
	}
	if claims.Role != "customer" {
		t.Fatalf("unexpected token role: got %q want %q", claims.Role, "customer")
	}
}

func TestRegisterAllowsMerchantRole(t *testing.T) {
	service, _ := newAuthTestService(t)

	resp, err := service.Register(context.Background(), &pb.RegisterRequest{
		Username: "merchant-user",
		Password: "password123",
		Email:    "merchant@example.com",
		Role:     "merchant",
	})
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if resp.Role != "merchant" {
		t.Fatalf("unexpected response role: got %q want %q", resp.Role, "merchant")
	}

	loginResp, err := service.Login(context.Background(), &pb.LoginRequest{
		Username: "merchant-user",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}
	if loginResp.Role != "merchant" {
		t.Fatalf("unexpected login role: got %q want %q", loginResp.Role, "merchant")
	}
}
