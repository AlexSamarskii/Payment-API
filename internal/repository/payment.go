package repository

import (
	"context"
	entity "paymentgo/internal/entity"
)

type PaymentRepository interface {
	CreatePayment(ctx context.Context, fromID, toID, currency string, amount float64) error
	GetPaymentByID(ctx context.Context, paymentID int) (*entity.Payment, error)
	GetPaymentHistory(ctx context.Context, userID string) ([]*entity.Payment, error)
	GetPaymentDetails(ctx context.Context, paymentID int) (float64, string, error)
	UpdatePaymentStatus(ctx context.Context, paymentID int, paymentStatus string) error
}
