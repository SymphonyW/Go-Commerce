package idempotency

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// HashPayload 基于稳定 JSON 编码生成请求摘要，屏蔽字段顺序差异。
func HashPayload(payload any) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}
