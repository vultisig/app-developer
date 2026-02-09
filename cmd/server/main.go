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
	"github.com/sirupsen/logrus"

	"github.com/vultisig/verifier/plugin"
	smetrics "github.com/vultisig/verifier/plugin/metrics"
	"github.com/vultisig/verifier/plugin/policy"
	"github.com/vultisig/verifier/plugin/policy/policy_pg"
	"github.com/vultisig/verifier/plugin/redis"
	"github.com/vultisig/verifier/plugin/scheduler"
	pluginserver "github.com/vultisig/verifier/plugin/server"
	"github.com/vultisig/verifier/vault"
	"golang.org/x/sync/errgroup"

	"github.com/vultisig/app-developer/internal/config"
	"github.com/vultisig/app-developer/internal/db"
	appserver "github.com/vultisig/app-developer/internal/server"
	"github.com/vultisig/app-developer/spec"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.ReadServerConfig()
	if err != nil {
		logrus.Fatalf("failed to load config: %v", err)
	}

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

	pgPool, err := pgxpool.New(ctx, cfg.Database.DSN)
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

	middlewares := pluginserver.DefaultMiddlewares(logger)

	srv := pluginserver.NewServer(
		cfg.Server,
		policyService,
		redisClient,
		vaultStorage,
		asynqClient,
		asynqInspector,
		spec.NewSpec(),
		middlewares,
		smetrics.NewNilPluginServerMetrics(),
		logger,
		nil,
	)
	if cfg.Verifier.Token != "" {
		srv.SetAuthMiddleware(pluginserver.NewAuth(cfg.Verifier.Token).Middleware)
	}

	e := srv.GetRouter()

	listingAPI := appserver.NewDeveloperAPI(policyService, pgBackend, cfg.Fee, asynqClient, logger)
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
