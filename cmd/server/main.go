package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kelseyhightower/envconfig"
	"github.com/sirupsen/logrus"

	"github.com/vultisig/verifier/plugin"
	plugin_config "github.com/vultisig/verifier/plugin/config"
	plugin_metrics "github.com/vultisig/verifier/plugin/metrics"
	"github.com/vultisig/verifier/plugin/policy"
	"github.com/vultisig/verifier/plugin/policy/policy_pg"
	"github.com/vultisig/verifier/plugin/redis"
	"github.com/vultisig/verifier/plugin/scheduler"
	plugin_server "github.com/vultisig/verifier/plugin/server"
	"github.com/vultisig/verifier/vault"
	"github.com/vultisig/verifier/vault_config"
	"golang.org/x/sync/errgroup"

	app_config "github.com/vultisig/app-developer/internal/config"
	"github.com/vultisig/app-developer/internal/db"
	app_server "github.com/vultisig/app-developer/internal/server"
	"github.com/vultisig/app-developer/spec"
)

type config struct {
	Server        plugin_server.Config
	TaskQueueName string `envconfig:"TASK_QUEUE_NAME" default:"default_queue"`
	Postgres      plugin_config.Database
	Redis         plugin_config.Redis
	BlockStorage  vault_config.BlockStorage
	Verifier      plugin_config.Verifier
	Fee           app_config.FeeConfig
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

	if cfg.Verifier.Token == "" {
		logrus.Fatal("VERIFIER_TOKEN is required")
	}

	cfg.Server.TaskQueueName = cfg.TaskQueueName

	logger := logrus.New()

	redisClient, err := redis.NewRedis(cfg.Redis)
	if err != nil {
		logger.Fatalf("failed to initialize Redis client: %v", err)
	}

	asynqConnOpt, err := asynq.ParseRedisURI(cfg.Redis.URI)
	if err != nil {
		logger.Fatalf("failed to parse redis URI: %v", err)
	}

	asynqClient := asynq.NewClient(asynqConnOpt)
	asynqInspector := asynq.NewInspector(asynqConnOpt)

	vaultStorage, err := vault.NewBlockStorageImp(cfg.BlockStorage)
	if err != nil {
		logger.Fatalf("failed to initialize vault storage: %v", err)
	}

	pgPool, err := pgxpool.New(ctx, cfg.Postgres.DSN)
	if err != nil {
		logger.Fatalf("failed to initialize Postgres pool: %v", err)
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

	middlewares := plugin_server.DefaultMiddlewares(logger)

	srv := plugin_server.NewServer(
		cfg.Server,
		policyService,
		redisClient,
		vaultStorage,
		asynqClient,
		asynqInspector,
		spec.NewSpec(cfg.Fee.VultTokenAddress, cfg.Fee.TreasuryAddress, cfg.Fee.Amount),
		middlewares,
		plugin_metrics.NewNilPluginServerMetrics(),
		logger,
		nil,
	)
	srv.SetAuthMiddleware(plugin_server.NewAuth(cfg.Verifier.Token).Middleware)

	e := srv.GetRouter()

	listingAPI := app_server.NewDeveloperAPI(pgBackend, cfg.Fee, logger)
	listingAPI.RegisterRoutes(e)

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logger.Info("received shutdown signal")
		cancel()
	}()

	eg := &errgroup.Group{}
	eg.Go(func() error {
		startErr := e.Start(fmt.Sprintf(":%d", cfg.Server.Port))
		if errors.Is(startErr, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("failed to start server: %w", startErr)
	})
	eg.Go(func() error {
		<-ctx.Done()
		logger.Info("shutting down server...")
		c, cancelShutdown := context.WithTimeout(context.Background(), time.Minute)
		defer cancelShutdown()
		return e.Shutdown(c)
	})

	err = eg.Wait()
	if err != nil {
		logger.Fatalf("server error: %v", err)
	}
}
