package main

import (
	"context"
	"embed"
	"fmt"
	"net"

	"paymentgo/internal/cmd/auth"
	"paymentgo/internal/cmd/convert"
	"paymentgo/internal/cmd/yoomoney"
	"paymentgo/internal/config"
	"paymentgo/internal/repository/postgres"
	paymentsDemon "paymentgo/internal/server_demon"
	"paymentgo/internal/transport/grpc/proto"
	handlers "paymentgo/internal/transport/http"
	"paymentgo/internal/usecase/service"
	db "paymentgo/utils/connector"
	log "paymentgo/utils/logger"

	"go.uber.org/zap"
	"google.golang.org/grpc"
)

var migrations embed.FS

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		panic(fmt.Errorf("Failed to load config: %v", err))
	}

	logger := log.NewLogger(cfg)
	defer logger.Sync()

	ctx := context.WithValue(context.Background(), "logger", logger)

	dbConn, err := db.NewPostgres(ctx, cfg, logger)
	if err != nil {
		logger.Fatal("Failed to initialize PostgreSQL", zap.Error(err))
	}

	defer func() {
		if dbConn != nil {
			dbConn.Close()
			logger.Info("Database connection closed")
		}
	}()

	if err := db.MigratePostgres(ctx, dbConn, logger, migrations); err != nil {
		logger.Fatal("Failed to apply migrations", zap.Error(err))
	}

	const grpcServerAddress = "localhost:8888"

	authClient, err := auth.NewAuthClient(grpcServerAddress)
	if err != nil {
		logger.Fatal("Failed to create AuthClient")
	}
	defer authClient.Close()

	rdb := db.InitRedis(cfg, logger)
	defer rdb.Close()

	paymentsQueue := db.NewPaymentsQueue()

	converter := convert.NewForexClient(cfg)
	paymentClient := yoomoney.NewYooMoneyClient(cfg)

	repo := postgres.NewPaymentRepository(dbConn, rdb, logger)
	svc := service.NewPaymentService(repo, logger, converter, paymentClient, paymentsQueue)

	demon := paymentsDemon.NewPaymentDemon(*svc, repo, paymentClient, paymentsQueue, logger, authClient)
	go demon.Start(ctx)

	grpcServer := grpc.NewServer()
	paymentHandler := handlers.NewPaymentHandler(svc, logger)
	proto.RegisterPaymentServiceServer(grpcServer, paymentHandler)

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Server.Port))
	if err != nil {
		logger.Fatal("Failed to start gRPC listener", zap.Error(err))
	}
	logger.Info(fmt.Sprintf("Starting gRPC server on port %d", cfg.Server.Port))
	if err := grpcServer.Serve(listener); err != nil {
		logger.Fatal("Failed to start gRPC server", zap.Error(err))
	}
}
