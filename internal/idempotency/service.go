package idempotency

import (
	"context"
	"errors"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

var (
	ErrConflict   = errors.New("idempotency key reused with different request")
	ErrInProgress = errors.New("idempotent request is still processing")
)

type Action string

const (
	ActionProceed Action = "proceed"
	ActionReplay  Action = "replay"
)

type BeginRequest struct {
	UserID         uint
	RequestPath    string
	IdempotencyKey string
	RequestHash    string
}

type BeginResult struct {
	Action Action
	Record *Record
}

type Service struct {
	db  *gorm.DB
	ttl time.Duration
	now func() time.Time
}

func NewService(db *gorm.DB, ttl time.Duration) *Service {
	return &Service{
		db:  db,
		ttl: ttl,
		now: time.Now,
	}
}

func (s *Service) Begin(ctx context.Context, req BeginRequest) (*BeginResult, error) {
	record := &Record{
		IdempotencyKey: req.IdempotencyKey,
		UserID:         req.UserID,
		RequestPath:    req.RequestPath,
		RequestHash:    req.RequestHash,
		State:          StateProcessing,
		ExpiredAt:      s.now().Add(s.ttl),
	}
	createErr := s.db.WithContext(ctx).Create(record).Error
	if createErr == nil {
		return &BeginResult{Action: ActionProceed, Record: record}, nil
	}

	var existing Record
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND request_path = ? AND idempotency_key = ?", req.UserID, req.RequestPath, req.IdempotencyKey).
		First(&existing).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, createErr
		}
		return nil, err
	}
	if existing.RequestHash != req.RequestHash {
		return nil, ErrConflict
	}
	if existing.State == StateProcessing {
		return nil, ErrInProgress
	}
	return &BeginResult{Action: ActionReplay, Record: &existing}, nil
}

func (s *Service) Complete(ctx context.Context, recordID uint, statusCode int, response proto.Message) error {
	body, err := protojson.Marshal(response)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).
		Model(&Record{}).
		Where("id = ?", recordID).
		Updates(map[string]any{
			"response_body": string(body),
			"status_code":   statusCode,
			"state":         StateCompleted,
		}).Error
}

// Abort 仅清理仍处于处理中状态的记录，供业务事务失败且没有副作用落库时释放幂等键。
func (s *Service) Abort(ctx context.Context, recordID uint) error {
	return s.db.WithContext(ctx).
		Where("id = ? AND state = ?", recordID, StateProcessing).
		Delete(&Record{}).Error
}

func ReplayInto(record *Record, response proto.Message) error {
	return protojson.Unmarshal([]byte(record.ResponseBody), response)
}
