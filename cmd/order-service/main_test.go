package main

import (
	"os"
	"strings"
	"testing"
)

func TestOrderServiceStartupMigratesIdempotencyRecords(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("failed to read order service main.go: %v", err)
	}

	if !strings.Contains(string(source), "&idempotency.Record{}") {
		t.Fatal("order service startup migration must include idempotency.Record")
	}
}
