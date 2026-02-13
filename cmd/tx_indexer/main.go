package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kelseyhightower/envconfig"
	"github.com/sirupsen/logrus"

	"github.com/vultisig/verifier/plugin"
	plugin_config "github.com/vultisig/verifier/plugin/config"
	plugin_metrics "github.com/vultisig/verifier/plugin/metrics"
	"github.com/vultisig/verifier/plugin/tx_indexer"
	tx_config "github.com/vultisig/verifier/plugin/tx_indexer/pkg/config"
	tx_storage "github.com/vultisig/verifier/plugin/tx_indexer/pkg/storage"

	"github.com/vultisig/app-developer/internal/health"
)

type config struct {
	Database         plugin_config.Database
	EthRpcURL        string        `envconfig:"ETH_RPC_URL" default:"https://ethereum-rpc.publicnode.com"`
	Interval         time.Duration `default:"15s"`
	IterationTimeout time.Duration `default:"60s"`
	MarkLostAfter    time.Duration `default:"30m"`
	Concurrency      int           `default:"5"`
	HealthPort       int           `default:"8083"`
}

func newConfig() (config, error) {
	var cfg config
	err := envconfig.Process("", &cfg)
	if err != nil {
		return config{}, fmt.Errorf("failed to process env var: %w", err)
	}
	return cfg, nil
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := newConfig()
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
		tx_storage.NewRepo,
		"tx_indexer/pkg/storage/migrations",
	)
	if err != nil {
		logger.Fatalf("failed to initialize tx_indexer storage: %v", err)
	}

	rpcCfg := tx_config.RpcConfig{
		Ethereum: tx_config.RpcItem{URL: cfg.EthRpcURL},
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
		plugin_metrics.NewNilTxIndexerMetrics(),
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
