package payment

import (
	"context"
	"errors"
	"time"

	pb "go-commerce/api/payment"

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
	payment, err := s.core.SucceedPayment(ctx, uint(req.UserId), uint(req.Id))
	if err != nil {
		return nil, paymentStatusError(err)
	}
	return &pb.PaymentActionResponse{Payment: convertToPBPayment(payment)}, nil
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
		Amount:        float32(payment.Amount),
		Status:        payment.Status,
		PaymentMethod: payment.PaymentMethod,
		CreatedAt:     payment.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     payment.UpdatedAt.Format(time.RFC3339),
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
