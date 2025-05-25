package connector

import (
	dto "paymentgo/internal/entity"
	"testing"
)

func TestLockFreeQueue_EnqueueDequeue(t *testing.T) {
	queue := NewPaymentsQueue()
	payment := dto.Payment{
		ID:     "1234",
		Amount: 100.0,
	}
	queue.Enqueue(payment)
	dequeuedPayment, ok := queue.Dequeue()
	if !ok {
		t.Errorf("Dequeue returned false, expected true")
	}
	if dequeuedPayment.ID != payment.ID {
		t.Errorf("Expected payment ID %s, but got %s", payment.ID, dequeuedPayment.ID)
	}
	if dequeuedPayment.Amount != payment.Amount {
		t.Errorf("Expected payment amount %f, but got %f", payment.Amount, dequeuedPayment.Amount)
	}
}

func TestLockFreeQueue_EnqueueListDequeue(t *testing.T) {
	queue := NewPaymentsQueue()
	payments := []dto.Payment{
		{ID: "1234", Amount: 100.0},
		{ID: "5678", Amount: 200.0},
		{ID: "9101", Amount: 300.0},
	}
	queue.EnqueueList(payments)
	for _, payment := range payments {
		dequeuedPayment, ok := queue.Dequeue()
		if !ok {
			t.Errorf("Dequeue returned false, expected true")
		}
		if dequeuedPayment.ID != payment.ID {
			t.Errorf("Expected payment ID %s, but got %s", payment.ID, dequeuedPayment.ID)
		}
		if dequeuedPayment.Amount != payment.Amount {
			t.Errorf("Expected payment amount %f, but got %f", payment.Amount, dequeuedPayment.Amount)
		}
	}
}

func TestLockFreeQueue_DequeueFromEmptyQueue(t *testing.T) {
	queue := NewPaymentsQueue()
	dequeuedPayment, ok := queue.Dequeue()
	if ok {
		t.Errorf("Dequeue returned true, expected false when queue is empty")
	}
	if dequeuedPayment != (dto.Payment{}) {
		t.Errorf("Expected empty payment, but got %+v", dequeuedPayment)
	}
}
