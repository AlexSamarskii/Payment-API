package server_demon

import (
	"context"
	"log"
	"paymentgo/internal/cmd/auth"
	"paymentgo/internal/cmd/yoomoney"
	dto "paymentgo/internal/entity"
	"paymentgo/internal/repository"
	"paymentgo/internal/usecase/service"
	db "paymentgo/utils/connector"
	"time"

	"go.uber.org/zap"
)

type PaymentDemon struct {
	service       service.PaymentService
	repo          repository.PaymentRepository
	paymentClient *yoomoney.YooMoneyClient
	paymentsQueue *db.LockFreeQueue
	authClient    *auth.AuthClient
	logger        *zap.Logger
}

func NewPaymentDemon(service service.PaymentService, repo repository.PaymentRepository, paymentClient *yoomoney.YooMoneyClient, paymentQueue *db.LockFreeQueue, logger *zap.Logger, authClient *auth.AuthClient) *PaymentDemon {
	return &PaymentDemon{
		service:       service,
		paymentClient: paymentClient,
		paymentsQueue: paymentQueue,
		logger:        logger,
		authClient:    authClient,
	}
}

// Start Бесконечный цикл проверки счетов
func (d PaymentDemon) Start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			log.Println("Payment demon stopped")
			return
		default:
			payment, ok := d.paymentsQueue.Dequeue()
			if !ok {
				time.Sleep(1 * time.Second)
				continue
			}

			status, err := d.service.GetPayment(ctx, payment.ID)
			if err != nil {
				d.paymentsQueue.Enqueue(payment)
				d.logger.Error("Failed to check payment status", zap.String("payment_id", payment.ID), zap.Error(err))
				continue
			}

			switch status {
			case "success":
				receiverData, err := d.authClient.GetUserById(ctx, payment.ToUserID)
				if err != nil {
					d.paymentsQueue.Enqueue(payment)
					d.logger.Error("Failed to get receiver", zap.String("user_id", payment.ToUserID), zap.Error(err))
				}

				receiver := receiverData.YoomoneyId

				err = d.repo.UpdatePaymentStatus(ctx, payment.ID, dto.StatusComplete)
				if err != nil {
					d.paymentsQueue.Enqueue(payment)
					d.logger.Error("Failed to update payment status", zap.String("payment_id", payment.ID), zap.Error(err))
				}

				paymentStatus, err := d.paymentClient.CreateTransfer(&payment, receiver)
				if err != nil {
					err = d.repo.UpdatePaymentStatus(ctx, payment.ID, dto.StatusSuccess)
					if err != nil {
						d.logger.Error("Failed to update payment status", zap.String("payment_id", payment.ID), zap.Error(err))
					}
					d.paymentsQueue.Enqueue(payment)
					d.logger.Error("Failed to create new transfer", zap.String("original_payment_id", payment.ID), zap.Error(err))
				} else {
					d.logger.Info("New transfer created", zap.String("status", paymentStatus))
				}

			case "pending", "failed":
				d.paymentsQueue.Enqueue(payment)
				d.logger.Info("Re-enqueued payment for further processing", zap.String("payment_id", payment.ID))
			case "complete":
				d.logger.Info("Payment complete", zap.String("payment_id", payment.ID))
			default:
				d.paymentsQueue.Enqueue(payment)
				d.logger.Warn("Unexpected payment status", zap.String("payment_id", payment.ID), zap.String("status", status))
			}
		}
	}
}
