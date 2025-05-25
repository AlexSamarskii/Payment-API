package dto

import "time"

type PaymentStatus string

const (
	StatusPending  PaymentStatus = "PENDING"
	StatusSuccess  PaymentStatus = "SUCCESS"
	StatusFailed   PaymentStatus = "FAILED"
	StatusRefunded PaymentStatus = "REFUNDED"
	StatusComplete PaymentStatus = "COMPLETE"
	CoreAccount                  = "4100118177295897"
)

type Payment struct {
	ID         string        `json:"id" db:"id"`
	FromUserID string        `json:"user_from_id" db:"from_user_id"`
	ToUserID   string        `json:"user_to_id" db:"to_user_id"`
	Amount     float64       `json:"amount" db:"amount"`
	Currency   string        `json:"currency" db:"currency"`
	Status     PaymentStatus `json:"status" db:"status"`
	CreatedAt  time.Time     `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time     `json:"updated_at" db:"updated_at"`
}

type PaymentDetails struct {
	Amount   float64 `json:"amount" db:"amount"`
	Currency string  `json:"currency" db:"currency"`
}
