package main

import (
	"context"
	"math/big"
	"os"
	"os/signal"
	"syscall"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sirupsen/logrus"

	"github.com/vultisig/verifier/plugin"
	"github.com/vultisig/verifier/plugin/keysign"
	"github.com/vultisig/verifier/plugin/policy"
	"github.com/vultisig/verifier/plugin/policy/policy_pg"
	"github.com/vultisig/verifier/plugin/scheduler"
	"github.com/vultisig/verifier/plugin/tasks"
	"github.com/vultisig/verifier/plugin/tx_indexer"
	txstorage "github.com/vultisig/verifier/plugin/tx_indexer/pkg/storage"
	"github.com/vultisig/verifier/vault"
	"github.com/vultisig/vultisig-go/relay"

	evmsdk "github.com/vultisig/recipes/sdk/evm"
	vcommon "github.com/vultisig/vultisig-go/common"

	"github.com/vultisig/app-developer/internal/config"
	"github.com/vultisig/app-developer/internal/db"
	"github.com/vultisig/app-developer/internal/evm"
	"github.com/vultisig/app-developer/internal/health"
	"github.com/vultisig/app-developer/internal/worker"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.ReadWorkerConfig()
	if err != nil {
		logrus.Fatalf("failed to load config: %v", err)
	}

	logger := logrus.New()

	vaultStorage, err := vault.NewBlockStorageImp(cfg.BlockStorage)
	if err != nil {
		logger.Fatalf("failed to initialize vault storage: %v", err)
	}

	asynqConnOpt, err := asynq.ParseRedisURI(cfg.Redis.URI)
	if err != nil {
		logger.Fatalf("failed to parse redis URI: %v", err)
	}

	asynqClient := asynq.NewClient(asynqConnOpt)

	queueName := cfg.TaskQueueName
	if queueName == "" {
		queueName = tasks.QUEUE_NAME
	}

	asynqServer := asynq.NewServer(
		asynqConnOpt,
		asynq.Config{
			Logger:      logger,
			Concurrency: 10,
			Queues: map[string]int{
				queueName: 10,
			},
		},
	)

	pgPool, err := pgxpool.New(ctx, cfg.Database.DSN)
	if err != nil {
		logger.Fatalf("failed to initialize Postgres pool: %v", err)
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

	vaultService, err := vault.NewManagementService(
		cfg.VaultServiceConfig,
		asynqClient,
		vaultStorage,
		txIndexerService,
		nil,
	)
	if err != nil {
		logger.Fatalf("failed to initialize vault service: %v", err)
	}

	policyStorage, err := plugin.WithMigrations(
		logger,
		pgPool,
		policy_pg.NewRepo,
		"policy/policy_pg/migrations",
	)
	if err != nil {
		logger.Fatalf("failed to initialize policy storage: %v", err)
	}

	policyService, err := policy.NewPolicyService(
		policyStorage,
		scheduler.NewNilService(),
		logger,
	)
	if err != nil {
		logger.Fatalf("failed to initialize policy service: %v", err)
	}

	pgBackend, err := db.NewPostgresBackend(logger, pgPool)
	if err != nil {
		logger.Fatalf("failed to initialize database: %v", err)
	}

	ethClient, err := ethclient.Dial(cfg.Fee.EthRpcURL)
	if err != nil {
		logger.Fatalf("failed to connect to Ethereum RPC: %v", err)
	}

	chainID := new(big.Int).SetUint64(cfg.Fee.ChainID)
	sdk := evmsdk.NewSDK(chainID, ethClient, ethClient.Client())

	signer := keysign.NewSigner(
		logger.WithField("pkg", "keysign.Signer").Logger,
		relay.NewRelayClient(cfg.VaultServiceConfig.Relay.Server),
		[]keysign.Emitter{
			keysign.NewPluginEmitter(asynqClient, tasks.TypeKeySignDKLS, queueName),
			keysign.NewVerifierEmitter(cfg.Verifier.URL, cfg.Verifier.Token),
		},
		[]string{
			cfg.VaultServiceConfig.LocalPartyPrefix,
			cfg.Verifier.PartyPrefix,
		},
	)

	signerService := evm.NewSignerService(sdk, vcommon.Ethereum, signer, txIndexerService)

	consumer := worker.NewConsumer(
		logger,
		policyService,
		signerService,
		sdk,
		pgBackend,
		vaultStorage,
		cfg.VaultServiceConfig.EncryptionSecret,
		cfg.Fee,
	)

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logger.Info("received shutdown signal")
		cancel()
	}()

	healthServer := health.New(cfg.HealthPort)
	go func() {
		healthErr := healthServer.Start(ctx, logger)
		if healthErr != nil {
			logger.Errorf("health server failed: %v", healthErr)
		}
	}()

	mux := asynq.NewServeMux()
	mux.HandleFunc(tasks.TypePluginTransaction, consumer.HandleExecuteListingFee)
	mux.HandleFunc(tasks.TypeKeySignDKLS, vaultService.HandleKeySignDKLS)
	mux.HandleFunc(tasks.TypeReshareDKLS, vaultService.HandleReshareDKLS)

	logger.Info("worker started")
	err = asynqServer.Run(mux)
	if err != nil {
		logger.Fatalf("failed to run worker: %v", err)
	}
}
