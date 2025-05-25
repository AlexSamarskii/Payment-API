# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Установка зависимостей для сборки
RUN apk add --no-cache git make gcc musl-dev

# Копируем только файлы, необходимые для загрузки зависимостей
COPY go.mod go.sum ./

# Загружаем зависимости
RUN go mod download

# Копируем весь исходный код
COPY . .

# Проверяем синтаксис перед сборкой
RUN go vet ./...

# Собираем приложение с правильным путем
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/bin/payment-service ./cmd/main.go

# Runtime stage
FROM alpine:3.18

WORKDIR /app

# Устанавливаем только необходимые пакеты
RUN apk add --no-cache ca-certificates tzdata

# Копируем бинарник и конфиги
COPY --from=builder /app/bin/payment-service /app/payment-service
COPY --from=builder /app/configs /app/configs

# Копируем статические файлы (если есть)
COPY --from=builder /app/internal /app/internal

EXPOSE 8080 50051

HEALTHCHECK --interval=30s --timeout=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

CMD ["/app/payment-service"]