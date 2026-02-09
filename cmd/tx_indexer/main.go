package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sirupsen/logrus"

	"github.com/vultisig/verifier/plugin"
	"github.com/vultisig/verifier/plugin/metrics"
	"github.com/vultisig/verifier/plugin/tx_indexer"
	txconfig "github.com/vultisig/verifier/plugin/tx_indexer/pkg/config"
	txstorage "github.com/vultisig/verifier/plugin/tx_indexer/pkg/storage"

	"github.com/vultisig/app-developer/internal/config"
	"github.com/vultisig/app-developer/internal/health"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.ReadTxIndexerConfig()
	if err != nil {
		logrus.Fatalf("failed to load config: %v", err)
	}

	logger := logrus.New()

	pgPool, err := pgxpool.New(ctx, cfg.Database.DSN)
	if err != nil {
		logger.Fatalf("failed to initialize Postgres pool: %v", err)
	}

	txStorage, err := plugin.WithMigrations(
		logger,
		pgPool,
		txstorage.NewRepo,
		"tx_indexer/pkg/storage/migrations",
	)
	if err != nil {
		logger.Fatalf("failed to initialize tx_indexer storage: %v", err)
	}

	rpcCfg := txconfig.RpcConfig{
		Ethereum: txconfig.RpcItem{URL: cfg.EthRpcURL},
	}

	rpcs, err := tx_indexer.Rpcs(ctx, rpcCfg)
	if err != nil {
		logger.Fatalf("failed to initialize RPCs: %v", err)
	}

	worker := tx_indexer.NewWorker(
		logger,
		cfg.Interval,
		cfg.IterationTimeout,
		cfg.MarkLostAfter,
		cfg.Concurrency,
		txStorage,
		rpcs,
		metrics.NewNilTxIndexerMetrics(),
	)

	healthServer := health.New(cfg.HealthPort)
	go func() {
		healthErr := healthServer.Start(ctx, logger)
		if healthErr != nil {
			logger.Errorf("health server failed: %v", healthErr)
		}
	}()

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logger.Info("received shutdown signal")
		cancel()
	}()

	logger.Info("tx_indexer started")
	err = worker.Run()
	if err != nil {
		logger.Fatalf("failed to run tx_indexer: %v", err)
	}
}
