package order

import (
	"testing"
	"time"
)

func TestParseOrderPaymentTimeoutMinutesSupportsDecimals(t *testing.T) {
	timeout, err := ParseOrderPaymentTimeoutMinutes("0.5")
	if err != nil {
		t.Fatalf("ParseOrderPaymentTimeoutMinutes returned error: %v", err)
	}
	if got, want := timeout, 30*time.Second; got != want {
		t.Fatalf("unexpected timeout: got %v want %v", got, want)
	}
}

func TestParseOrderPaymentTimeoutMinutesUsesDefaultWhenEmpty(t *testing.T) {
	timeout, err := ParseOrderPaymentTimeoutMinutes("")
	if err != nil {
		t.Fatalf("ParseOrderPaymentTimeoutMinutes returned error: %v", err)
	}
	if got, want := timeout, DefaultOrderPaymentTimeout; got != want {
		t.Fatalf("unexpected timeout: got %v want %v", got, want)
	}
}
