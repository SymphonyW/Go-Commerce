package payment

import (
	"context"
	"errors"
	"net/http"
	"time"

	pb "go-commerce/api/payment"
	"go-commerce/internal/idempotency"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GRPCService 暴露支付服务接口，核心校验仍委托给 Service。
type GRPCService struct {
	pb.UnimplementedPaymentServiceServer
	core *Service
}

func NewGRPCService(core *Service) *GRPCService {
	return &GRPCService{core: core}
}

func (s *GRPCService) CreatePayment(ctx context.Context, req *pb.CreatePaymentRequest) (*pb.CreatePaymentResponse, error) {
	payment, err := s.core.CreatePayment(ctx, req.UserId, req.OrderId, req.PaymentMethod)
	if err != nil {
		return nil, paymentStatusError(err)
	}
	return &pb.CreatePaymentResponse{Payment: convertToPBPayment(payment)}, nil
}

func (s *GRPCService) GetPayment(ctx context.Context, req *pb.GetPaymentRequest) (*pb.GetPaymentResponse, error) {
	payment, err := s.core.GetPayment(req.UserId, req.Id)
	if err != nil {
		return nil, paymentStatusError(err)
	}
	return &pb.GetPaymentResponse{Payment: convertToPBPayment(payment)}, nil
}

func (s *GRPCService) MarkPaymentSucceeded(ctx context.Context, req *pb.PaymentActionRequest) (*pb.PaymentActionResponse, error) {
	if req.IdempotencyKey == "" {
		return nil, status.Error(codes.InvalidArgument, "idempotency key required")
	}

	requestHash, err := idempotency.HashPayload(newPaymentSuccessFingerprint(req.UserId, req.Id))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to hash payment success request: %v", err)
	}

	idempotencyResult, err := s.core.idempotency.Begin(ctx, idempotency.BeginRequest{
		UserID:         uint(req.UserId),
		RequestPath:    paymentSuccessRequestPath,
		IdempotencyKey: req.IdempotencyKey,
		RequestHash:    requestHash,
	})
	if err != nil {
		return nil, paymentIdempotencyError(err)
	}
	if idempotencyResult.Action == idempotency.ActionReplay {
		var replay pb.PaymentActionResponse
		if err := idempotency.ReplayInto(idempotencyResult.Record, &replay); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to replay payment success response: %v", err)
		}
		return &replay, nil
	}

	payment, err := s.core.SucceedPayment(ctx, uint(req.UserId), uint(req.Id))
	if err != nil {
		_ = s.core.idempotency.Abort(ctx, idempotencyResult.Record.ID)
		return nil, paymentStatusError(err)
	}
	response := &pb.PaymentActionResponse{Payment: convertToPBPayment(payment)}
	if err := s.core.idempotency.Complete(ctx, idempotencyResult.Record.ID, http.StatusOK, response); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to finalize payment success idempotency record: %v", err)
	}
	return response, nil
}

func (s *GRPCService) MarkPaymentFailed(ctx context.Context, req *pb.PaymentActionRequest) (*pb.PaymentActionResponse, error) {
	payment, err := s.core.FailPayment(uint(req.UserId), uint(req.Id))
	if err != nil {
		return nil, paymentStatusError(err)
	}
	return &pb.PaymentActionResponse{Payment: convertToPBPayment(payment)}, nil
}

func convertToPBPayment(payment *Payment) *pb.Payment {
	return &pb.Payment{
		Id:            int64(payment.ID),
		PaymentNo:     payment.PaymentNo,
		OrderId:       int64(payment.OrderID),
		UserId:        int64(payment.UserID),
		AmountCents:   payment.AmountCents,
		Status:        payment.Status,
		PaymentMethod: payment.PaymentMethod,
		CreatedAt:     payment.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     payment.UpdatedAt.Format(time.RFC3339),
	}
}

const (
	paymentSuccessRequestPath = "/api/payments/:id/success"
	paymentSuccessAction      = "payment_success"
)

type paymentSuccessFingerprint struct {
	Action    string `json:"action"`
	UserID    int64  `json:"user_id"`
	PaymentID int64  `json:"payment_id"`
}

func newPaymentSuccessFingerprint(userID, paymentID int64) paymentSuccessFingerprint {
	return paymentSuccessFingerprint{
		Action:    paymentSuccessAction,
		UserID:    userID,
		PaymentID: paymentID,
	}
}

func paymentStatusError(err error) error {
	switch {
	case errors.Is(err, ErrInvalidPaymentMethod):
		return status.Error(codes.InvalidArgument, "invalid payment method")
	case errors.Is(err, ErrOrderNotFound), errors.Is(err, ErrPaymentNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, ErrOrderNotPayable), errors.Is(err, ErrActivePaymentExists), errors.Is(err, ErrPaymentNotActionable):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		return status.Errorf(codes.Internal, "payment operation failed: %v", err)
	}
}

func paymentIdempotencyError(err error) error {
	switch {
	case errors.Is(err, idempotency.ErrConflict):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, idempotency.ErrInProgress):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		return status.Errorf(codes.Internal, "idempotency operation failed: %v", err)
	}
}
