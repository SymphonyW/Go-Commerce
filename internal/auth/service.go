// 认证服务：处理用户注册、登录和令牌验证
package auth

import (
	"context"
	"errors"

	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"

	pb "go-commerce/api/auth"
	"go-commerce/pkg/jwt"
)

// Service 认证服务结构体

type Service struct {
	pb.UnimplementedAuthServiceServer
	db *gorm.DB // 数据库连接
}

// NewService 创建认证服务实例
func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// Register 用户注册：创建新用户并生成JWT令牌
func (s *Service) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	// 检查用户名是否已存在
	var existingUser User
	if err := s.db.Where("username = ?", req.Username).First(&existingUser).Error; err == nil {
		return nil, status.Errorf(codes.AlreadyExists, "username already exists")
	}

	// 密码哈希处理
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to hash password")
	}

	// 创建新用户
	user := User{
		Username: req.Username,
		Password: string(hashedPassword),
		Email:    req.Email,
	}
	if err := s.db.Create(&user).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create user")
	}

	// 生成JWT令牌
	token, err := jwt.GenerateToken(int64(user.ID))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate token")
	}

	return &pb.RegisterResponse{
		UserId: int64(user.ID),
		Token:  token,
	}, nil
}

// Login 用户登录：验证用户凭据并生成JWT令牌
func (s *Service) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	// 查找用户
	var user User
	if err := s.db.Where("username = ?", req.Username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(codes.NotFound, "user not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to query user")
	}

	// 验证密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "invalid password")
	}

	// 生成JWT令牌
	token, err := jwt.GenerateToken(int64(user.ID))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate token")
	}

	return &pb.LoginResponse{
		UserId: int64(user.ID),
		Token:  token,
	}, nil
}

// ValidateToken 验证令牌：检查JWT令牌的有效性
func (s *Service) ValidateToken(ctx context.Context, req *pb.ValidateTokenRequest) (*pb.ValidateTokenResponse, error) {
	// 验证令牌
	claims, err := jwt.ValidateToken(req.Token)
	if err != nil {
		return &pb.ValidateTokenResponse{Valid: false}, nil
	}

	return &pb.ValidateTokenResponse{
		Valid:  true,
		UserId: claims.UserID,
	}, nil
}
