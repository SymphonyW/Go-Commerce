package order

import (
	"errors"
	"fmt"
)

const (
	OrderStatusPending   = "pending"
	OrderStatusPaid      = "paid"
	OrderStatusShipped   = "shipped"
	OrderStatusCompleted = "completed"
	OrderStatusCancelled = "cancelled"
)

var ErrInvalidOrderTransition = errors.New("invalid order status transition")

var allowedTransitions = map[string]map[string]struct{}{
	OrderStatusPending: {
		OrderStatusPaid:      {},
		OrderStatusCancelled: {},
	},
	OrderStatusPaid: {
		OrderStatusShipped: {},
	},
	OrderStatusShipped: {
		OrderStatusCompleted: {},
	},
}

// CanTransition 只回答状态机层面的合法性，不承载权限与业务副作用。
func CanTransition(from, to string) bool {
	nextStatuses, ok := allowedTransitions[from]
	if !ok {
		return false
	}
	_, ok = nextStatuses[to]
	return ok
}

// ValidateTransition 为所有状态变更提供统一入口，避免业务代码各自手写判断。
func ValidateTransition(from, to string) error {
	if CanTransition(from, to) {
		return nil
	}
	return fmt.Errorf("%w: %s -> %s", ErrInvalidOrderTransition, from, to)
}

// TransitionTo 在写入前执行统一校验，订单业务不得绕过该函数直接改状态。
func TransitionTo(order *Order, nextStatus string) error {
	if err := ValidateTransition(order.Status, nextStatus); err != nil {
		return err
	}
	order.Status = nextStatus
	return nil
}
