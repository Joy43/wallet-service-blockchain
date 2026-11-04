// Command auth is the authentication microservice. It exposes:
//   - HTTP  (register / login / refresh / logout / me) for clients
//   - gRPC  (ValidateToken) for other services
//   - a background worker consuming auth.> events from NATS JetStream
//
// State: users in Postgres; sessions, rate limits and the user cache in Redis.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	authv1 "github.com/yourorg/wallet-service/gen/auth/v1"
	"github.com/yourorg/wallet-service/services/auth/internal/config"
	"github.com/yourorg/wallet-service/services/auth/internal/controller"
	"github.com/yourorg/wallet-service/services/auth/internal/events"
	"github.com/yourorg/wallet-service/services/auth/internal/repository/postgres"
	"github.com/yourorg/wallet-service/services/auth/internal/repository/redisstore"
	"github.com/yourorg/wallet-service/services/auth/internal/router"
	"github.com/yourorg/wallet-service/services/auth/internal/security"
	"github.com/yourorg/wallet-service/services/auth/internal/service"
	"github.com/yourorg/wallet-service/services/auth/internal/worker"
)

func main() {
	if err := run(); err != nil {
		slog.Error("auth-service stopped with error", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadAuth()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Postgres
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("create db pool: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping db: %w", err)
	}

	// Redis
	ropts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return fmt.Errorf("parse REDIS_URL: %w", err)
	}
	rdb := redis.NewClient(ropts)
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("ping redis: %w", err)
	}

	// NATS JetStream: stream + durable consumer for the audit worker
	nc, err := nats.Connect(cfg.NATSURL, nats.Name("auth-service"))
	if err != nil {
		return fmt.Errorf("connect nats: %w", err)
	}
	defer nc.Drain()
	js, err := jetstream.New(nc)
	if err != nil {
		return fmt.Errorf("create jetstream: %w", err)
	}
	if _, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name: events.AuthStream, Subjects: []string{events.AuthSubjectFilter},
	}); err != nil {
		return fmt.Errorf("ensure AUTH stream: %w", err)
	}
	consumer, err := js.CreateOrUpdateConsumer(ctx, events.AuthStream, jetstream.ConsumerConfig{
		Durable:       "auth-audit-worker",
		FilterSubject: events.AuthSubjectFilter,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       30 * time.Second,
		MaxDeliver:    10,
	})
	if err != nil {
		return fmt.Errorf("ensure audit consumer: %w", err)
	}

	// Wire layers
	issuer, err := security.NewJWTIssuer(cfg.JWTPrivateKey, cfg.JWTKeyID, cfg.JWTIssuer, cfg.AccessTTL)
	if err != nil {
		return err
	}
	authPublisher := events.NewAuthPublisher(js)
	authSvc, err := service.NewAuthService(
		postgres.NewUserRepository(pool),
		redisstore.NewUserCache(rdb),
		redisstore.NewSessionStore(rdb),
		redisstore.NewRateLimiter(rdb),
		issuer,
		security.NewBcryptHasher(cfg.BcryptCost),
		authPublisher,
		service.AuthConfig{
			RefreshTTL: cfg.RefreshTTL, UserCacheTTL: cfg.UserCacheTTL,
			LoginLimit: cfg.LoginLimit, LoginWindow: cfg.LoginWindow,
		}, logger)
	if err != nil {
		return err
	}
	outboxRelay := worker.NewOutboxRelay(postgres.NewOutboxRepository(pool), authPublisher.PublishRaw, time.Second, logger)
	auditWorker := worker.NewAuthWorker(consumer, postgres.NewAuditRepository(pool), cfg.AuditRetention, logger)

	ready := func(ctx context.Context) error {
		if err := pool.Ping(ctx); err != nil {
			return fmt.Errorf("db: %w", err)
		}
		if err := rdb.Ping(ctx).Err(); err != nil {
			return fmt.Errorf("redis: %w", err)
		}
		if !nc.IsConnected() {
			return errors.New("nats: not connected")
		}
		return nil
	}

	httpSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router.NewAuthRouter(authSvc, logger, ready),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	grpcSrv := grpc.NewServer()
	authv1.RegisterAuthServiceServer(grpcSrv, controller.NewGRPCAuthController(authSvc))
	// Standard gRPC health protocol: Kubernetes probes and client-side load balancers use it.
	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(grpcSrv, healthSrv)
	lis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		return fmt.Errorf("listen grpc: %w", err)
	}

	// Run everything; the first failure (or a signal) cancels the rest.
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		logger.Info("http server listening", slog.String("addr", cfg.HTTPAddr))
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("http server: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		logger.Info("grpc server listening", slog.String("addr", cfg.GRPCAddr))
		if err := grpcSrv.Serve(lis); err != nil {
			return fmt.Errorf("grpc server: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		logger.Info("outbox relay started")
		if err := outboxRelay.Run(gctx); err != nil {
			return fmt.Errorf("outbox relay: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		logger.Info("audit worker started")
		if err := auditWorker.Run(gctx); err != nil {
			return fmt.Errorf("audit worker: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		<-gctx.Done()
		logger.Info("shutting down")
		healthSrv.Shutdown() // report NOT_SERVING first so callers stop sending traffic
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		grpcDone := make(chan struct{})
		go func() { grpcSrv.GracefulStop(); close(grpcDone) }()
		select {
		case <-grpcDone:
		case <-shutdownCtx.Done():
			grpcSrv.Stop()
		}
		return httpSrv.Shutdown(shutdownCtx)
	})
	return g.Wait()
}
