// jwt 包提供JWT令牌的生成和验证功能
// 用于用户认证和授权
package jwt

import (
	"time"

	// JWT库：用于生成和解析JWT令牌
	"github.com/golang-jwt/jwt/v5"
)

// secretKey JWT签名密钥
// 注意：生产环境需从环境变量读取，此处为示例
var secretKey = []byte("your-secret-key")

// Claims JWT声明结构体
// 包含用户ID和标准JWT声明

type Claims struct {
	UserID int64 `json:"user_id"` // 用户ID
	jwt.RegisteredClaims           // 嵌入标准JWT声明
}

// GenerateToken 生成JWT令牌
// 参数：
//   userID: 用户ID
// 返回值：
//   string: JWT令牌字符串
//   error: 错误信息
func GenerateToken(userID int64) (string, error) {
	// 构建JWT声明
	claims := Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)), // 令牌过期时间：24小时
			IssuedAt:  jwt.NewNumericDate(time.Now()),                   // 令牌签发时间
			Issuer:    "go-ecommerce",                                 // 令牌签发者
		},
	}

	// 创建令牌
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	// 签名令牌并返回
	return token.SignedString(secretKey)
}

// ValidateToken 验证JWT令牌
// 参数：
//   tokenString: JWT令牌字符串
// 返回值：
//   *Claims: JWT声明，包含用户ID
//   error: 错误信息
func ValidateToken(tokenString string) (*Claims, error) {
	// 初始化声明结构体
	claims := &Claims{}
	// 解析令牌
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return secretKey, nil
	})

	// 处理解析错误
	if err != nil {
		return nil, err
	}

	// 验证令牌有效性
	if !token.Valid {
		return nil, jwt.ErrSignatureInvalid
	}

	// 返回声明
	return claims, nil
}
