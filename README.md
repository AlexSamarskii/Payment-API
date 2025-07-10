# Payment Gateway Service

Сервис обработки платежей на Go, предоставляющий API для:

- Проведения платежей и возвратов  
- Конвертации валют  
- Интеграции с платежными системами (например, YooMoney)  
- Управления историей платежей  

---

## Технологический стек

**Язык:**  
- Go 1.20+

**Фреймворки:**  
- gRPC — для API  
- Gin — для HTTP-роутинга  

**Базы данных:**  
- PostgreSQL — основное хранилище  
- Redis — кеширование и очереди  

**Интеграции:**  
- [YooMoney API](https://yoomoney.ru/) — прием и возврат платежей  
- [FastForex API](https://fastforex.io/) — курсы валют и конвертация  

**Инструменты:**  
- Zap — логирование  
- Viper — конфигурация  
- GORM — ORM для работы с базой данных  
- Docker — контейнеризация и развертывание  

---

## Архитектура

Сервис построен по многослойной архитектуре:
```text
┌───────────────────────┐
│       API Layer       │  (gRPC / HTTP handlers)
└───────────┬───────────┘
            │
┌───────────▼───────────┐
│    Service Layer       │  (Бизнес-логика)
└───────────┬───────────┘
            │
┌───────────▼───────────┐
│  Repository Layer      │  (Работа с БД)
└───────────┬───────────┘
            │
┌───────────▼───────────┐
│ External Integrations  │  (YooMoney, FastForex)
└───────────────────────┘
```
---

## Основные функции

### Управление платежами
- Создание платежей  
- Проверка статусов  
- Возвраты  
- История операций  

### Конвертация валют 
- Кеширование данных  

### Безопасность
- JWT-аутентификация  
- Шифрование чувствительных данных  
- Валидация входящих запросов  

---
## API ендпоинты
```proto
service PaymentService {
  // Создание нового платежа
  rpc CreatePayment (CreatePaymentRequest) returns (CreatePaymentResponse);
  
  // Получение статуса платежа
  rpc GetPayment (GetPaymentRequest) returns (GetPaymentResponse);
  
  // Получение полной информации о платеже
  rpc GetPaymentByID (GetPaymentByIDRequest) returns (GetPaymentByIDResponse);
  
  // Возврат платежа
  rpc RefundPayment (RefundPaymentRequest) returns (RefundPaymentResponse);
  
  // Получение истории платежей пользователя
  rpc GetPaymentHistory (GetPaymentHistoryRequest) returns (GetPaymentHistoryResponse);
  
  // Получение ссылки на оплату
  rpc GetPaymentLink (GetPaymentLinkRequest) returns (GetPaymentLinkResponse);
  
  // Получение активных платежей пользователя
  rpc GetActivePayments (GetActivePaymentsRequest) returns (GetActivePaymentsResponse);
}
```
---
## Запуск сервиса

### Требования

- Docker `20.10+`  
- Docker Compose `1.29+`  
- Go `1.24` (для локальной разработки)

### Быстрый старт

```bash
docker-compose up -d
```
### Формат .env файла
```.env
SERVER_PORT=50051

POSTGRES_HOST=postgres
POSTGRES_PORT=5432
POSTGRES_SSL_MODE=disable
POSTGRES_DB=payment
POSTGRES_USER=postgres
POSTGRES_PASSWORD=supersecretpassword123

REDIS_URL=redis:6379

FOREX_KEY=fx_demo_1234567890abcdef

YOOMONEY_TOKEN=41001111223344556677889900aabbccddeeff
YOOMONEY_CLIENT_ID=1234567890ABCDEF1234567890ABCDEF
YOOMONEY_RECEIVER=4100111122223333
```