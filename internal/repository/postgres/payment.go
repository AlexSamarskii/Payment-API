package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	entity "paymentgo/internal/entity"
	"paymentgo/internal/repository"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type PaymentRepository struct {
	DB     *pgxpool.Pool
	redis  *redis.Client
	logger *zap.Logger
}

func NewPaymentRepository(db *pgxpool.Pool, redis *redis.Client, logger *zap.Logger) repository.PaymentRepository {
	return &PaymentRepository{
		DB:     db,
		redis:  redis,
		logger: logger,
	}
}

func (pr *PaymentRepository) CreatePayment(ctx context.Context, fromID, toID, currency string, amount float64) (string, error) {
	id := uuid.New().String()
	query := `INSERT INTO payment 
	(id, user_from_id, user_to_id, status, currency, amount, created_at, updated_at) 
			  VALUES ($1, $2, $3, 'PENDING', $4, $5, NOW(), NOW()) 
			  RETURNING id`
	var paymentID string
	err := pr.DB.QueryRow(ctx, query, id, fromID, toID, currency, amount).Scan(&paymentID)
	if err != nil {
		return "", fmt.Errorf("error creating payment: %w", err)
	}
	return paymentID, nil

}

func (pr *PaymentRepository) GetPaymentByID(ctx context.Context, paymentID string) (*entity.Payment, error) {
	cacheKey := fmt.Sprintf("payment:%s", paymentID)

	cachedPayment, err := pr.redis.Get(ctx, cacheKey).Result()
	if err == nil {
		var payment entity.Payment
		if err := json.Unmarshal([]byte(cachedPayment), &payment); err == nil {
			return &payment, nil
		}
	}
	query := `SELECT id, user_from_id, user_to_id, status, currency, amount, created_at, updated_at
				FROM payment WHERE id=$1`
	var payment entity.Payment
	err = pr.DB.QueryRow(ctx, query, paymentID).Scan(
		&payment.ID,
		&payment.FromUserID,
		&payment.ToUserID,
		&payment.Status,
		&payment.Currency,
		&payment.Amount,
		&payment.CreatedAt,
		&payment.UpdatedAt,
	)
	if err != nil {
		pr.logger.Error("Failed to fetch payment by ID", zap.String("payment_id", paymentID), zap.Error(err))
		return nil, fmt.Errorf("error fetching payment by ID: %w", err)
	}

	data, err := json.Marshal(payment)
	if err == nil {
		pr.redis.Set(ctx, cacheKey, data, 10*time.Minute)
	}

	return &payment, nil
}

func (pr *PaymentRepository) GetPaymentDetails(ctx context.Context, paymentID string) (float64, string, error) {
	cacheKey := fmt.Sprintf("payment_details:%s", paymentID)

	cachedDetails, err := pr.redis.Get(ctx, cacheKey).Result()
	if err == nil {
		var details struct {
			Amount   float64 `json:"amount"`
			Currency string  `json:"currency"`
		}
		if err := json.Unmarshal([]byte(cachedDetails), &details); err == nil {
			return details.Amount, details.Currency, nil
		}
	}

	query := `SELECT amount, currency FROM payment WHERE id = $1`
	var amount float64
	var currency string
	err = pr.DB.QueryRow(ctx, query, paymentID).Scan(&amount, &currency)
	if err != nil {
		pr.logger.Error("Failed to fetch payment details", zap.String("payment_id", paymentID), zap.Error(err))
		return 0, "", fmt.Errorf("error fetching payment details: %w", err)
	}

	details := entity.PaymentDetails{Amount: amount, Currency: currency}
	data, err := json.Marshal(details)
	if err == nil {
		pr.redis.Set(ctx, cacheKey, data, 10*time.Minute)
	}

	return amount, currency, nil
}

func (pr *PaymentRepository) UpdatePaymentStatus(ctx context.Context, paymentID string, status entity.PaymentStatus) error {
	query := `UPDATE payment SET status = $1, updated_at = $2 WHERE id = $3`
	_, err := pr.DB.Exec(ctx, query, status, time.Now(), paymentID)
	if err != nil {
		pr.logger.Error("Failed to update payment status", zap.String("payment_id", paymentID), zap.String("status", string(status)), zap.Error(err))
		return fmt.Errorf("error updating payment status: %w", err)
	}

	return nil
}

func (pr *PaymentRepository) GetPaymentHistory(ctx context.Context, userID string, page, limit int) ([]*entity.Payment, error) {
	cacheKey := fmt.Sprintf("payment_history:%s:%d:%d", userID)

	cachedHistory, err := pr.redis.Get(ctx, cacheKey).Result()
	if err == nil {
		var payments []*entity.Payment
		if err := json.Unmarshal([]byte(cachedHistory), &payments); err == nil {
			return payments, nil
		}
	}

	query := `SELECT id, user_from_id, user_to_id, status, currency, amount, created_at, updated_at 
			  FROM payment WHERE user_from_id = $1
			  LIMIT $1 OFFSET $2`
	rows, err := pr.DB.Query(ctx, query, userID, page, limit)
	if err != nil {
		return nil, fmt.Errorf("error fetching payment history: %w", err)
	}
	defer rows.Close()

	var payments []*entity.Payment
	for rows.Next() {
		var payment entity.Payment
		if err := rows.Scan(
			&payment.ID,
			&payment.FromUserID,
			&payment.ToUserID,
			&payment.Amount,
			&payment.Currency,
			&payment.Status,
			&payment.CreatedAt,
			&payment.UpdatedAt,
		); err != nil {
			pr.logger.Error("Failed to scan payment history row", zap.Error(err))
			return nil, fmt.Errorf("error scanning payment history: %w", err)
		}
		payments = append(payments, &payment)
	}

	data, err := json.Marshal(payments)
	if err == nil {
		pr.redis.Set(ctx, cacheKey, data, 10*time.Minute)
	}

	return payments, nil
}

func (r *PaymentRepository) GetActivePayments(ctx context.Context, userID string) ([]*entity.Payment, error) {
	query := `SELECT id, from_user_id, to_user_id, amount, currency, status, created_at, updated_at 
			  FROM payments WHERE from_user_id = $1 AND status = 'PENDING' OR from_user_id = $1 AND status = 'FAILED'`

	rows, err := r.DB.Query(ctx, query, userID)
	if err != nil {
		r.logger.Error("Failed to fetch payment history", zap.String("user_id", userID), zap.Error(err))
		return nil, fmt.Errorf("error fetching payment history: %w", err)
	}
	defer rows.Close()

	var payments []*entity.Payment
	for rows.Next() {
		var payment entity.Payment
		if err := rows.Scan(
			&payment.ID,
			&payment.FromUserID,
			&payment.ToUserID,
			&payment.Amount,
			&payment.Currency,
			&payment.Status,
			&payment.CreatedAt,
			&payment.UpdatedAt,
		); err != nil {
			r.logger.Error("Failed to scan payment history row", zap.Error(err))
			return nil, fmt.Errorf("error scanning payment history: %w", err)
		}
		payments = append(payments, &payment)
	}

	if err := rows.Err(); err != nil {
		r.logger.Error("Error occurred during rows iteration", zap.Error(err))
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return payments, nil
}
