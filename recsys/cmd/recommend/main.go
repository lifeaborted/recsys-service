package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	recommendationv1 "recsys/api/recommendation/v1"
	"recsys/internal/config"
	"recsys/internal/contract"
	"recsys/internal/logger"
	"recsys/internal/recommend"
	"recsys/internal/store/postgres"
	"recsys/internal/transport/recommendgrpc"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
)

func main() {
	log := logger.New(os.Stdout, slog.LevelInfo)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, config.Load(), log); err != nil {
		log.Error("gRPC recommendation service stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	creds, err := recommendgrpc.ServerCredentials(cfg.GRPC)
	if err != nil {
		return err
	}
	startupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	store, err := postgres.OpenContext(startupCtx, cfg.DatabaseURL, postgres.WithOnlineSources(os.Getenv("RECSYS_ONLINE_SNAPSHOT"), os.Getenv("EMBED_MODEL")), postgres.WithClosedPreview(os.Getenv("RECSYS_CLOSED_PREVIEW") == "1"))
	if err != nil {
		return err
	}
	defer store.Close()
	if _, err := store.CheckHealth(startupCtx); err != nil {
		return fmt.Errorf("recommendation database is not ready: %w", err)
	}

	engine := recommend.New(store)
	warmupCtx, cancelWarmup := context.WithCancel(ctx)
	defer cancelWarmup()
	if os.Getenv("RECSYS_ONLINE_SNAPSHOT") != "" {
		go func() {
			warmupCtx, cancel := context.WithTimeout(warmupCtx, cfg.RequestTimeout)
			defer cancel()
			_, err := engine.Recommend(warmupCtx, contract.RecommendationQuery{ClientID: "DEMO-GUEST", Mode: contract.Online, Limit: 3})
			if err != nil {
				log.Debug("guest recommendations warmup skipped", "error", err)
			}
		}()
	}

	listener, err := net.Listen("tcp", cfg.GRPC.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen for recommendation RPCs: %w", err)
	}
	defer listener.Close()
	var options []grpc.ServerOption
	if creds != nil {
		options = append(options, grpc.Creds(creds))
	}
	server := grpc.NewServer(options...)
	recommendationv1.RegisterRecommendationServiceServer(server, recommendgrpc.NewServer(engine, cfg.RequestTimeout, log))
	healthServer := health.NewServer()
	healthServer.SetServingStatus(recommendationv1.RecommendationService_ServiceDesc.ServiceName, healthv1.HealthCheckResponse_SERVING)
	healthv1.RegisterHealthServer(server, healthServer)

	log.Info("starting gRPC recommendation service", "addr", listener.Addr().String(), "mutual_tls", creds != nil)
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			return err
		}
	case <-ctx.Done():
		healthServer.Shutdown()
		timer := time.AfterFunc(10*time.Second, server.Stop)
		defer timer.Stop()
		server.GracefulStop()
	}
	return nil
}
