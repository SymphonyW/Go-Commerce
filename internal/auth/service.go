// auth 包包含认证服务的模型和业务逻辑
// 负责处理用户注册、登录和令牌验证
package auth

import (
	"context"
	"errors"

	// bcrypt：用于密码哈希处理
	"golang.org/x/crypto/bcrypt"
	// gRPC状态码：用于返回标准化的错误信息
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	// GORM：ORM框架，用于数据库操作
	"gorm.io/gorm"

	// 导入认证服务的protobuf生成代码
	pb "go-commerce/api/auth"
	// JWT工具：用于生成和验证JWT令牌
	"go-commerce/pkg/jwt"
)

// Service 认证服务结构体
// 实现了AuthServiceServer接口

type Service struct {
	pb.UnimplementedAuthServiceServer // 嵌入未实现的AuthServiceServer，以保持向后兼容性
	db *gorm.DB                       // 数据库连接
}

// NewService 创建认证服务实例
// 参数：
//   db: 数据库连接
// 返回值：
//   *Service: 认证服务实例
func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// Register 用户注册：创建新用户并生成JWT令牌
// 参数：
//   ctx: 上下文，用于控制请求的生命周期
//   req: 注册请求，包含用户名、密码和邮箱
// 返回值：
//   *pb.RegisterResponse: 注册响应，包含用户ID和JWT令牌
//   error: 错误信息
func (s *Service) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	// 检查用户名是否已存在
	var existingUser User
	if err := s.db.Where("username = ?", req.Username).First(&existingUser).Error; err == nil {
		return nil, status.Errorf(codes.AlreadyExists, "username already exists")
	}

	// 密码哈希处理
	// 使用bcrypt算法对密码进行哈希，增强安全性
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
	// 用于后续的身份验证
	token, err := jwt.GenerateToken(int64(user.ID))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate token")
	}

	// 返回注册响应
	return &pb.RegisterResponse{
		UserId: int64(user.ID),
		Token:  token,
	}, nil
}

// Login 用户登录：验证用户凭据并生成JWT令牌
// 参数：
//   ctx: 上下文，用于控制请求的生命周期
//   req: 登录请求，包含用户名和密码
// 返回值：
//   *pb.LoginResponse: 登录响应，包含用户ID和JWT令牌
//   error: 错误信息
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
	// 比较输入的密码与数据库中存储的哈希值
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "invalid password")
	}

	// 生成JWT令牌
	// 用于后续的身份验证
	token, err := jwt.GenerateToken(int64(user.ID))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate token")
	}

	// 返回登录响应
	return &pb.LoginResponse{
		UserId: int64(user.ID),
		Token:  token,
	}, nil
}

// ValidateToken 验证令牌：检查JWT令牌的有效性
// 参数：
//   ctx: 上下文，用于控制请求的生命周期
//   req: 验证令牌请求，包含JWT令牌
// 返回值：
//   *pb.ValidateTokenResponse: 验证响应，包含令牌是否有效和用户ID
//   error: 错误信息
func (s *Service) ValidateToken(ctx context.Context, req *pb.ValidateTokenRequest) (*pb.ValidateTokenResponse, error) {
	// 验证令牌
	claims, err := jwt.ValidateToken(req.Token)
	if err != nil {
		// 令牌无效，返回false
		return &pb.ValidateTokenResponse{Valid: false}, nil
	}

	// 令牌有效，返回true和用户ID
	return &pb.ValidateTokenResponse{
		Valid:  true,
		UserId: claims.UserID,
	}, nil
}
