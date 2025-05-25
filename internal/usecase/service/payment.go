package service

import (
	"context"
	"fmt"
	"paymentgo/internal/repository"

	"go.uber.org/zap"

	convert "paymentgo/internal/cmd/convert"
	yoomoney "paymentgo/internal/cmd/yoomoney"
	dto "paymentgo/internal/entity"
	db "paymentgo/utils/connector"
)

// PaymentService структура для сервиса
type PaymentService struct {
	repo          repository.PaymentRepository
	logger        *zap.Logger
	converter     *convert.ForexClient
	paymentClient *yoomoney.YooMoneyClient
	paymentsQueue *db.LockFreeQueue
}

// NewPaymentService создание экземпляра сервиса
func NewPaymentService(repo repository.PaymentRepository, logger *zap.Logger, converter *convert.ForexClient, paymentClient *clients.YooMoneyClient, paymentsQueue *db.LockFreeQueue) *PaymentService {
	return &PaymentService{
		repo:          repo,
		logger:        logger,
		converter:     converter,
		paymentClient: paymentClient,
		paymentsQueue: paymentsQueue,
	}
}

// GetPaymentLink создание ссылки для оплаты
func (s *PaymentService) GetPaymentLink(ctx context.Context, paymentID string) (string, error) {
	s.logger.Info("Getting payment link", zap.String("payment_id", paymentID))

	amount, currency, err := s.repo.GetPaymentDetails(ctx, paymentID)
	if err != nil {
		return "", fmt.Errorf("failed to get payment details: %w", err)
	}

	convertedAmount, err := s.converter.ConvertToRub(amount, currency)
	if err != nil {
		return "", fmt.Errorf("failed to convert amount: %w", err)
	}

	err = s.repo.UpdatePaymentStatus(ctx, paymentID, dto.StatusPending)
	if err != nil {
		return "", fmt.Errorf("error changing payment status to pending: %w", err)
	}

	payment, err := s.repo.GetPaymentByID(ctx, paymentID)
	if err != nil {
		s.logger.Error("Failed to fetch payment by ID", zap.String("payment_id", paymentID), zap.Error(err))
		return "", fmt.Errorf("error fetching payment: %w", err)
	}

	link, err := s.paymentClient.QuickPayment(dto.CoreAccount, paymentID, "AC", convertedAmount, paymentID, paymentID, paymentID, "")
	if err != nil {
		s.logger.Error("Failed to create payment link", zap.String("payment_id", paymentID), zap.Error(err))
		return "", fmt.Errorf("error creating payment link: %w", err)
	}

	s.paymentsQueue.Enqueue(*payment)

	return link, nil
}

func (s *PaymentService) GetPayment(ctx context.Context, paymentID string) (string, error) {
	s.logger.Info("Getting payment", zap.String("payment_id", paymentID))

	status, err := s.paymentClient.CheckPaymentStatus(paymentID)
	if err != nil {
		s.logger.Error("Failed to check payment status", zap.String("payment_id", paymentID), zap.Error(err))
		return "error", fmt.Errorf("error getting payment status: %w", err)
	}

	switch status {
	case "success":
		payment, err := s.repo.GetPaymentByID(ctx, paymentID)
		if err != nil {
			return "error", fmt.Errorf("error fetching payment: %w", err)
		}
		if payment.Status != "COMPLETE" {
			err := s.repo.UpdatePaymentStatus(ctx, paymentID, dto.StatusSuccess)
			if err != nil {
				return "error", fmt.Errorf("error changing payment status to success: %w", err)
			}
		} else {
			return "complete", nil
		}
	case "pending":
		err := s.repo.UpdatePaymentStatus(ctx, paymentID, dto.StatusPending)
		if err != nil {
			return "pending", nil
		}
	case "failed":
		err := s.repo.UpdatePaymentStatus(ctx, paymentID, dto.StatusFailed)
		if err != nil {
			return "error", fmt.Errorf("error changing payment status to failed: %w", err)
		}
	}

	s.logger.Info("GetPayment: ", zap.String("payment_status", status))
	return status, nil
}

func (s *PaymentService) CreatePayment(ctx context.Context, fromUserID, toUserID string, amount float64, currency string) (string, error) {
	s.logger.Info("Creating payment", zap.String("user_id", fromUserID), zap.Float64("amount", amount), zap.String("currency", currency))

	paymentID, err := s.repo.CreatePayment(ctx, fromUserID, toUserID, currency, amount)
	if err != nil {
		s.logger.Error("Failed to create payment", zap.Error(err))
		return "", err
	}

	s.logger.Info("Payment created successfully", zap.String("payment_id", paymentID))
	return paymentID, nil
}

func (s *PaymentService) RefundPayment(ctx context.Context, paymentID string) error {
	s.logger.Info("Refunding payment", zap.String("payment_id", paymentID))

	payment, err := s.repo.GetPaymentByID(ctx, paymentID)
	if err != nil {
		s.logger.Error("Failed to get payment by ID", zap.String("payment_id", paymentID), zap.Error(err))
		return fmt.Errorf("error fetching payment by ID: %w", err)
	}

	if payment.Status != "COMPLETE" {
		err = s.repo.UpdatePaymentStatus(ctx, paymentID, "REFUNDED")
		if err != nil {
			return fmt.Errorf("error updating payment status: %w", err)
		}
		newPaymentID, err := s.repo.CreatePayment(ctx, payment.ToUserID, payment.FromUserID, payment.Currency, payment.Amount)
		if err != nil {
			s.logger.Error("Failed to create new payment", zap.String("payment_id", newPaymentID), zap.Error(err))
			return fmt.Errorf("error creating payment: %w", err)
		}

		s.logger.Info("Payment refund process initiated successfully", zap.String("new_payment_id", newPaymentID))
		return nil
	}
	return fmt.Errorf("payment has not been paid")
}

func (s *PaymentService) GetPaymentByID(ctx context.Context, paymentID string) (*dto.Payment, error) {
	s.logger.Info("Getting payment by ID", zap.String("payment_id", paymentID))

	payment, err := s.repo.GetPaymentByID(ctx, paymentID)
	if err != nil {
		s.logger.Error("Failed to get payment", zap.String("payment_id", paymentID), zap.Error(err))
		return nil, err
	}

	s.logger.Info("Payment retrieved", zap.String("payment_id", paymentID))
	return payment, nil
}

func (s *PaymentService) GetPaymentHistory(ctx context.Context, userID string, page, limit int) ([]*dto.Payment, error) {
	s.logger.Info("Getting payment history", zap.String("user_id", userID), zap.Int("page", page), zap.Int("limit", limit))

	payments, err := s.repo.GetPaymentHistory(ctx, userID, page, limit)
	if err != nil {
		s.logger.Error("Failed to get payment history", zap.String("user_id", userID), zap.Error(err))
		return nil, err
	}

	s.logger.Info("Payment history retrieved", zap.String("user_id", userID))
	return payments, nil
}

func (s *PaymentService) UpdatePaymentStatus(ctx context.Context, paymentID string, status dto.PaymentStatus) error {
	s.logger.Info("Updating payment status", zap.String("payment_id", paymentID), zap.String("status", string(status)))

	err := s.repo.UpdatePaymentStatus(ctx, paymentID, status)
	if err != nil {
		s.logger.Error("Failed to update payment status", zap.String("payment_id", paymentID), zap.String("status", string(status)), zap.Error(err))
		return err
	}

	s.logger.Info("Payment status updated successfully", zap.String("payment_id", paymentID), zap.String("status", string(status)))
	return nil
}

// GetActivePayments Получение активных счетов пользователя
func (s *PaymentService) GetActivePayments(ctx context.Context, userID string) ([]*dto.Payment, error) {
	s.logger.Info("Getting active payments", zap.String("user_id", userID))

	activePayments, err := s.repo.GetActivePayments(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active payments: %w", err)
	}

	return activePayments, nil
}
