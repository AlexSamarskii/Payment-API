package repository

import (
	"context"
	entity "paymentgo/internal/entity"
)

type PaymentRepository interface {
	CreatePayment(ctx context.Context, fromID, toID, currency string, amount float64) (string, error)
	GetPaymentByID(ctx context.Context, paymentID string) (*entity.Payment, error)
	GetPaymentHistory(ctx context.Context, userID string, page, limit int) ([]*entity.Payment, error)
	GetPaymentDetails(ctx context.Context, paymentID string) (float64, string, error)
	UpdatePaymentStatus(ctx context.Context, paymentID string, paymentStatus entity.PaymentStatus) error
	GetActivePayments(ctx context.Context, userID string) ([]*entity.Payment, error)
}
