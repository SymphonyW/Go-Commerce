package order

import "testing"

func TestCanTransition(t *testing.T) {
	tests := []struct {
		name string
		from string
		to   string
		want bool
	}{
		{name: "pending to paid", from: OrderStatusPending, to: OrderStatusPaid, want: true},
		{name: "pending to cancelled", from: OrderStatusPending, to: OrderStatusCancelled, want: true},
		{name: "paid to shipped", from: OrderStatusPaid, to: OrderStatusShipped, want: true},
		{name: "shipped to completed", from: OrderStatusShipped, to: OrderStatusCompleted, want: true},
		{name: "cancelled to paid", from: OrderStatusCancelled, to: OrderStatusPaid, want: false},
		{name: "paid to completed", from: OrderStatusPaid, to: OrderStatusCompleted, want: false},
		{name: "completed to cancelled", from: OrderStatusCompleted, to: OrderStatusCancelled, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := CanTransition(tc.from, tc.to); got != tc.want {
				t.Fatalf("unexpected transition result: got %v want %v", got, tc.want)
			}
		})
	}
}

func TestValidateTransitionReturnsClearError(t *testing.T) {
	err := ValidateTransition(OrderStatusPaid, OrderStatusCompleted)
	if err == nil {
		t.Fatal("expected invalid transition error")
	}
	if got, want := err.Error(), "invalid order status transition: paid -> completed"; got != want {
		t.Fatalf("unexpected error message: got %q want %q", got, want)
	}
}
