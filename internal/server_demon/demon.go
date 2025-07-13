package server_demon

import (
	"context"
	"log"
	"paymentgo/internal/cmd/auth"
	"paymentgo/internal/cmd/yoomoney"
	dto "paymentgo/internal/entity"
	"paymentgo/internal/repository"
	"paymentgo/internal/usecase/service"
	"paymentgo/utils/connector"
	"time"

	"go.uber.org/zap"
)

type Daemon struct {
	paymentService service.PaymentService
	storage        repository.PaymentRepository
	yooClient      *yoomoney.Client
	taskQueue      *connector.LockFreeQueue
	authService    *auth.AuthClient
	log            *zap.Logger
}

func NewDaemon(paymentService service.PaymentService, storage repository.PaymentRepository, yooClient *yoomoney.Client, taskQueue *connector.LockFreeQueue, log *zap.Logger, authService *auth.AuthClient) *Daemon {
	return &Daemon{
		paymentService: paymentService,
		storage:        storage,
		yooClient:      yooClient,
		taskQueue:      taskQueue,
		log:            log,
		authService:    authService,
	}
}

// Run запускает постоянную обработку платежей
func (d Daemon) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			log.Println("Payment daemon gracefully stopped.")
			return
		default:
			d.processNext(ctx)
		}
	}
}

func (d Daemon) processNext(ctx context.Context) {
	item, ok := d.taskQueue.Dequeue()
	if !ok {
		time.Sleep(time.Second)
		return
	}

	payment := item

	status, err := d.paymentService.GetPayment(ctx, payment.ID)
	if err != nil {
		d.taskQueue.Enqueue(payment)
		d.log.Error("Unable to fetch payment status", zap.String("payment_id", payment.ID), zap.Error(err))
		return
	}

	switch status {
	case "success":
		d.handleSuccess(ctx, payment)
	case "pending", "failed":
		d.taskQueue.Enqueue(payment)
		d.log.Info("Payment returned to queue", zap.String("payment_id", payment.ID), zap.String("status", status))
	case "complete":
		d.log.Info("Payment already completed", zap.String("payment_id", payment.ID))
	default:
		d.taskQueue.Enqueue(payment)
		d.log.Warn("Unknown status received", zap.String("payment_id", payment.ID), zap.String("status", status))
	}
}

func (d Daemon) handleSuccess(ctx context.Context, payment dto.Payment) {
	user, err := d.authService.GetUserById(ctx, payment.ToUserID)
	if err != nil {
		d.taskQueue.Enqueue(payment)
		d.log.Error("Receiver lookup failed", zap.String("user_id", payment.ToUserID), zap.Error(err))
		return
	}

	if err := d.storage.UpdatePaymentStatus(ctx, payment.ID, dto.StatusComplete); err != nil {
		d.taskQueue.Enqueue(payment)
		d.log.Error("Failed to mark payment as complete", zap.String("payment_id", payment.ID), zap.Error(err))
		return
	}

	result, err := d.yooClient.InitiateTransfer(&payment, user.YoomoneyId)
	if err != nil {
		_ = d.storage.UpdatePaymentStatus(ctx, payment.ID, dto.StatusSuccess)
		d.taskQueue.Enqueue(payment)
		d.log.Error("Transfer initiation failed", zap.String("payment_id", payment.ID), zap.Error(err))
		return
	}

	d.log.Info("Transfer initiated successfully", zap.String("status", result))
}
