package observability

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// NewRequestID 生成足够短、可读、又不容易碰撞的请求标识。
func NewRequestID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("req-%d", time.Now().UTC().UnixNano())
	}
	return hex.EncodeToString(buf)
}
