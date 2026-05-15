package jwt

import "testing"

func TestGenerateTokenPreservesRole(t *testing.T) {
	token, err := GenerateToken(42, "merchant")
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	claims, err := ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken returned error: %v", err)
	}
	if claims.UserID != 42 {
		t.Fatalf("unexpected user id: got %d want %d", claims.UserID, 42)
	}
	if claims.Role != "merchant" {
		t.Fatalf("unexpected role: got %q want %q", claims.Role, "merchant")
	}
}
