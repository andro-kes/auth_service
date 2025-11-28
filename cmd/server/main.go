package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/andro-kes/auth/internal/logger"
	"github.com/andro-kes/auth/internal/rpc"
	pb "github.com/andro-kes/auth/proto"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

func main() {
	// logger init
	cfg := logger.Config{
		Level:        "debug",
		Encoding:     "console",
		FileRotation: false,
		Development: true,
	}
	if err := logger.Init(cfg); err != nil {
		_, _ = os.Stderr.WriteString("failed to init logger: " + err.Error())
		os.Exit(1)
	}
	
	// gRPC server init
	addr := os.Getenv("GRPC_ADDR")
	listen, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Logger().Fatal("Cannot listen tcp", zap.Error(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	pool, err := NewPool(ctx)
	defer func() {
		cancel()
		pool.Close()
		logger.Sync()
	}()

	rpcAuth := rpc.NewAuthServer(ctx, pool)
	grpcServer := grpc.NewServer()
	pb.RegisterAuthServiceServer(grpcServer, rpcAuth)

	go func() {
		if err := grpcServer.Serve(listen); err != nil {
			logger.Logger().Fatal("Error by serving", zap.Error(err))
		}
	}()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)
	<-shutdown

	grpcServer.GracefulStop()
}

func NewPool(ctx context.Context) (*pgxpool.Pool, error) {
	dbURL := os.Getenv("DB_URL")
	cfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		return nil, err
	}

	cfg.MaxConns = 20
	cfg.MinConns = 2
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.HealthCheckPeriod = 1 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}

	attempts := 3
	delay := time.Second
	for i := 0; i < attempts; i++ {
		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := pool.Ping(pingCtx)
		cancel()
		if err == nil {
			return pool, nil
		}
		time.Sleep(delay)
		delay *= 2
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}