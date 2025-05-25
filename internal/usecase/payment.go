package usecase

import (
	"context"
	entity "paymentgo/internal/entity"
)

type Payment interface {
	GetPaymentLink(ctx context.Context, paymentID string) (string, error)
	GetPayment(ctx context.Context, paymentID string) (string, error)
	CreatePayment(ctx context.Context, fromUserID, toUserID string, amount float64, currency string) (string, error)
	GetPaymentByID(ctx context.Context, paymentID string) (*entity.Payment, error)
	GetPaymentHistory(ctx context.Context, userID string, page, limit int) ([]*entity.Payment, error)
	UpdatePaymentStatus(ctx context.Context, paymentID string, status entity.PaymentStatus) error
	GetActivePayments(ctx context.Context, userID string) ([]*entity.Payment, error)
}
