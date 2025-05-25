package connector

import (
	"paymentgo/internal/config"

	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

// InitRedis создание редиски
func InitRedis(cfg *config.Config, logger *zap.Logger) *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr: cfg.Redis.URL,
	})
	logger.Info("Redis connected")
	return rdb
}
