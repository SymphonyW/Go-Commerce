package idempotency

import (
	"context"
	"errors"
	"time"

	gomysql "github.com/go-sql-driver/mysql"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

var (
	ErrConflict       = errors.New("idempotency key reused with different request")
	ErrInProgress     = errors.New("idempotent request is still processing")
	ErrTakeoverFailed = errors.New("expired idempotency record takeover failed")
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
	now := s.now()
	expiredAt := now.Add(s.ttl)
	record := &Record{
		IdempotencyKey: req.IdempotencyKey,
		UserID:         req.UserID,
		RequestPath:    req.RequestPath,
		RequestHash:    req.RequestHash,
		State:          StateProcessing,
		ExpiredAt:      expiredAt,
	}
	createErr := s.db.WithContext(ctx).Create(record).Error
	if createErr == nil {
		return &BeginResult{Action: ActionProceed, Record: record}, nil
	}
	if !isUniqueConstraintError(createErr) {
		return nil, createErr
	}

	return s.beginExisting(ctx, req, now, expiredAt)
}

func (s *Service) beginExisting(ctx context.Context, req BeginRequest, now, expiredAt time.Time) (*BeginResult, error) {
	result := s.db.WithContext(ctx).
		Model(&Record{}).
		Where("user_id = ? AND request_path = ? AND idempotency_key = ? AND expired_at <= ?", req.UserID, req.RequestPath, req.IdempotencyKey, now).
		Updates(map[string]any{
			"request_hash":  req.RequestHash,
			"response_body": "",
			"status_code":   0,
			"state":         StateProcessing,
			"expired_at":    expiredAt,
		})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 1 {
		record, err := s.findRecord(ctx, req)
		if err != nil {
			return nil, err
		}
		return &BeginResult{Action: ActionProceed, Record: record}, nil
	}
	if result.RowsAffected > 1 {
		return nil, ErrTakeoverFailed
	}

	existing, err := s.findRecord(ctx, req)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTakeoverFailed
		}
		return nil, err
	}
	if !existing.ExpiredAt.After(now) {
		return nil, ErrTakeoverFailed
	}
	if existing.RequestHash != req.RequestHash {
		return nil, ErrConflict
	}
	if existing.State == StateProcessing {
		return nil, ErrInProgress
	}
	if existing.State == StateCompleted {
		return &BeginResult{Action: ActionReplay, Record: existing}, nil
	}
	return nil, ErrTakeoverFailed
}

func (s *Service) findRecord(ctx context.Context, req BeginRequest) (*Record, error) {
	var existing Record
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND request_path = ? AND idempotency_key = ?", req.UserID, req.RequestPath, req.IdempotencyKey).
		First(&existing).Error; err != nil {
		return nil, err
	}
	return &existing, nil
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
		Unscoped().
		Where("id = ? AND state = ?", recordID, StateProcessing).
		Delete(&Record{}).Error
}

func (s *Service) DeleteExpiredBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	result := s.db.WithContext(ctx).
		Unscoped().
		Where("expired_at < ?", cutoff).
		Delete(&Record{})
	return result.RowsAffected, result.Error
}

func ReplayInto(record *Record, response proto.Message) error {
	return protojson.Unmarshal([]byte(record.ResponseBody), response)
}

func isUniqueConstraintError(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}

	var mysqlErr *gomysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1062
	}

	var sqliteErr interface{ Code() int }
	if errors.As(err, &sqliteErr) {
		switch sqliteErr.Code() {
		case 1555, 2067:
			return true
		}
	}

	return false
}
