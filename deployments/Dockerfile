FROM golang:1.24-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git make gcc musl-dev

COPY go.mod go.sum ./
RUN go mod download

# Устанавливаем goose и копируем как migrate
RUN go install github.com/pressly/goose/v3/cmd/goose@latest && \
    cp /go/bin/goose /go/bin/migrate

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/payment-service ./app/main.go

FROM alpine:3.18

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata postgresql-client

# Копируем с явными путями
COPY --from=builder /app/payment-service .
COPY --from=builder /go/bin/migrate /usr/bin/
COPY --from=builder /app/migrations ./migrations

# Проверяем доступность migrate
RUN migrate -version

EXPOSE 8080 50051

CMD ["sh", "-c", "migrate -dir ./migrations -database \"postgres://$POSTGRES_USER:$POSTGRES_PASSWORD@$POSTGRES_HOST:$POSTGRES_PORT/$POSTGRES_DB?sslmode=$POSTGRES_SSL_MODE\" up && ./payment-service"]