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
	db     *pgxpool.Pool // unexported field
	redis  *redis.Client
	logger *zap.Logger
}

func NewPaymentRepository(db *pgxpool.Pool, redis *redis.Client, logger *zap.Logger) repository.PaymentRepository {
	return &PaymentRepository{
		db:     db,
		redis:  redis,
		logger: logger.With(zap.String("component", "payment_repository")),
	}
}

func (pr *PaymentRepository) CreatePayment(ctx context.Context, fromID, toID, currency string, amount float64) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	id := uuid.New().String()
	query := `INSERT INTO payment 
	(id, user_from_id, user_to_id, status, currency, amount, created_at, updated_at) 
	VALUES ($1, $2, $3, 'PENDING', $4, $5, NOW(), NOW()) 
	RETURNING id`

	var paymentID string
	err := pr.db.QueryRow(ctx, query, id, fromID, toID, currency, amount).Scan(&paymentID)
	if err != nil {
		pr.logger.Error("failed to create payment",
			zap.String("from_id", fromID),
			zap.String("to_id", toID),
			zap.String("currency", currency),
			zap.Float64("amount", amount),
			zap.Error(err))
		return "", fmt.Errorf("failed to create payment: %w", err)
	}

	return paymentID, nil
}

func (pr *PaymentRepository) GetPaymentByID(ctx context.Context, paymentID string) (*entity.Payment, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	cacheKey := fmt.Sprintf("payment:%s", paymentID)

	// Try cache first
	cachedPayment, err := pr.redis.Get(ctx, cacheKey).Result()
	if err == nil {
		var payment entity.Payment
		if err := json.Unmarshal([]byte(cachedPayment), &payment); err == nil {
			return &payment, nil
		}
		pr.logger.Warn("failed to unmarshal cached payment", zap.Error(err))
	}

	query := `SELECT id, user_from_id, user_to_id, status, currency, amount, created_at, updated_at
	FROM payment WHERE id=$1`

	var payment entity.Payment
	err = pr.db.QueryRow(ctx, query, paymentID).Scan(
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
		pr.logger.Error("failed to fetch payment by ID",
			zap.String("payment_id", paymentID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to fetch payment %s: %w", paymentID, err)
	}

	// Update cache
	if data, err := json.Marshal(payment); err == nil {
		if err := pr.redis.Set(ctx, cacheKey, data, 10*time.Minute).Err(); err != nil {
			pr.logger.Warn("failed to cache payment",
				zap.String("payment_id", paymentID),
				zap.Error(err))
		}
	}

	return &payment, nil
}

func (pr *PaymentRepository) UpdatePaymentStatus(ctx context.Context, paymentID string, status entity.PaymentStatus) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	tx, err := pr.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	query := `UPDATE payment SET status = $1, updated_at = NOW() WHERE id = $2`
	if _, err := tx.Exec(ctx, query, status, paymentID); err != nil {
		pr.logger.Error("failed to update payment status",
			zap.String("payment_id", paymentID),
			zap.String("status", string(status)),
			zap.Error(err))
		return fmt.Errorf("failed to update payment status: %w", err)
	}

	// Invalidate cache
	cacheKey := fmt.Sprintf("payment:%s", paymentID)
	if err := pr.redis.Del(ctx, cacheKey).Err(); err != nil {
		pr.logger.Warn("failed to invalidate payment cache",
			zap.String("payment_id", paymentID),
			zap.Error(err))
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func (pr *PaymentRepository) GetPaymentHistory(ctx context.Context, userID string, page, limit int) ([]*entity.Payment, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cacheKey := fmt.Sprintf("payment_history:%s:%d:%d", userID, page, limit)

	// Try cache first
	if cachedHistory, err := pr.redis.Get(ctx, cacheKey).Result(); err == nil {
		var payments []*entity.Payment
		if err := json.Unmarshal([]byte(cachedHistory), &payments); err == nil {
			return payments, nil
		}
		pr.logger.Warn("failed to unmarshal cached payment history", zap.Error(err))
	}

	offset := (page - 1) * limit
	query := `SELECT id, user_from_id, user_to_id, status, currency, amount, created_at, updated_at 
	FROM payment WHERE user_from_id = $1 OR user_to_id = $1
	ORDER BY created_at DESC
	LIMIT $2 OFFSET $3`

	rows, err := pr.db.Query(ctx, query, userID, limit, offset)
	if err != nil {
		pr.logger.Error("failed to query payment history",
			zap.String("user_id", userID),
			zap.Int("page", page),
			zap.Int("limit", limit),
			zap.Error(err))
		return nil, fmt.Errorf("failed to query payment history: %w", err)
	}
	defer rows.Close()

	var payments []*entity.Payment
	for rows.Next() {
		var payment entity.Payment
		if err := rows.Scan(
			&payment.ID,
			&payment.FromUserID,
			&payment.ToUserID,
			&payment.Status,
			&payment.Currency,
			&payment.Amount,
			&payment.CreatedAt,
			&payment.UpdatedAt,
		); err != nil {
			pr.logger.Error("failed to scan payment history row", zap.Error(err))
			return nil, fmt.Errorf("failed to scan payment row: %w", err)
		}
		payments = append(payments, &payment)
	}

	if err := rows.Err(); err != nil {
		pr.logger.Error("payment history rows error", zap.Error(err))
		return nil, fmt.Errorf("rows error: %w", err)
	}

	// Update cache
	if data, err := json.Marshal(payments); err == nil {
		if err := pr.redis.Set(ctx, cacheKey, data, 10*time.Minute).Err(); err != nil {
			pr.logger.Warn("failed to cache payment history",
				zap.String("user_id", userID),
				zap.Error(err))
		}
	}

	return payments, nil
}

func (pr *PaymentRepository) GetActivePayments(ctx context.Context, userID string) ([]*entity.Payment, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	query := `SELECT id, user_from_id, user_to_id, status, currency, amount, created_at, updated_at 
	FROM payment 
	WHERE (user_from_id = $1 OR user_to_id = $1) 
	AND status IN ('PENDING', 'FAILED')
	ORDER BY created_at DESC`

	rows, err := pr.db.Query(ctx, query, userID)
	if err != nil {
		pr.logger.Error("failed to fetch active payments",
			zap.String("user_id", userID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to fetch active payments: %w", err)
	}
	defer rows.Close()

	var payments []*entity.Payment
	for rows.Next() {
		var payment entity.Payment
		if err := rows.Scan(
			&payment.ID,
			&payment.FromUserID,
			&payment.ToUserID,
			&payment.Status,
			&payment.Currency,
			&payment.Amount,
			&payment.CreatedAt,
			&payment.UpdatedAt,
		); err != nil {
			pr.logger.Error("failed to scan active payment row", zap.Error(err))
			return nil, fmt.Errorf("failed to scan payment row: %w", err)
		}
		payments = append(payments, &payment)
	}

	if err := rows.Err(); err != nil {
		pr.logger.Error("active payments rows error", zap.Error(err))
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return payments, nil
}

func (pr *PaymentRepository) GetPaymentDetails(ctx context.Context, paymentID string) (float64, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	cacheKey := fmt.Sprintf("payment_details:%s", paymentID)

	// Try cache first
	if cachedDetails, err := pr.redis.Get(ctx, cacheKey).Result(); err == nil {
		var details struct {
			Amount   float64 `json:"amount"`
			Currency string  `json:"currency"`
		}
		if err := json.Unmarshal([]byte(cachedDetails), &details); err == nil {
			return details.Amount, details.Currency, nil
		}
		pr.logger.Warn("failed to unmarshal cached payment details",
			zap.String("payment_id", paymentID),
			zap.Error(err))
	}

	query := `SELECT amount, currency FROM payment WHERE id = $1`
	var (
		amount   float64
		currency string
	)
	err := pr.db.QueryRow(ctx, query, paymentID).Scan(&amount, &currency)
	if err != nil {
		pr.logger.Error("failed to fetch payment details",
			zap.String("payment_id", paymentID),
			zap.Error(err))
		return 0, "", fmt.Errorf("failed to get payment details for %s: %w", paymentID, err)
	}

	// Update cache
	details := struct {
		Amount   float64 `json:"amount"`
		Currency string  `json:"currency"`
	}{amount, currency}

	if data, err := json.Marshal(details); err == nil {
		if err := pr.redis.Set(ctx, cacheKey, data, 10*time.Minute).Err(); err != nil {
			pr.logger.Warn("failed to cache payment details",
				zap.String("payment_id", paymentID),
				zap.Error(err))
		}
	}

	return amount, currency, nil
}
