package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sirupsen/logrus"

	"github.com/vultisig/verifier/plugin"
	"github.com/vultisig/verifier/plugin/tx_indexer"
	txstorage "github.com/vultisig/verifier/plugin/tx_indexer/pkg/storage"

	"github.com/vultisig/app-developer/internal/config"
	"github.com/vultisig/app-developer/internal/db"
	"github.com/vultisig/app-developer/internal/health"
	"github.com/vultisig/app-developer/internal/syncer"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.ReadSyncerConfig()
	if err != nil {
		logrus.Fatalf("failed to load config: %v", err)
	}

	logger := logrus.New()

	pgPool, err := pgxpool.New(ctx, cfg.Database.DSN)
	if err != nil {
		logger.Fatalf("failed to initialize Postgres pool: %v", err)
	}

	pgBackend, err := db.NewPostgresBackend(logger, pgPool)
	if err != nil {
		logger.Fatalf("failed to initialize database: %v", err)
	}

	txIndexerStorage, err := plugin.WithMigrations(
		logger,
		pgPool,
		txstorage.NewRepo,
		"tx_indexer/pkg/storage/migrations",
	)
	if err != nil {
		logger.Fatalf("failed to initialize tx_indexer storage: %v", err)
	}

	supportedChains, err := tx_indexer.Chains()
	if err != nil {
		logger.Fatalf("failed to initialize supported chains: %v", err)
	}

	txIndexerService := tx_indexer.NewService(logger, txIndexerStorage, supportedChains)

	txSyncer := syncer.NewTxSyncer(txIndexerService, pgBackend, logger, cfg.SyncerInterval)

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

	logger.Info("syncer started")
	txSyncer.Run(ctx)
}
