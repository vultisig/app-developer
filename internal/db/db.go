package db

import (
	"embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/sirupsen/logrus"
)

//go:embed migrations/developer/*.sql
var developerMigrations embed.FS

type PostgresBackend struct {
	pool *pgxpool.Pool
}

func NewPostgresBackend(logger *logrus.Logger, pool *pgxpool.Pool) (*PostgresBackend, error) {
	mgr := &DeveloperMigrationManager{pool: pool}
	err := mgr.Migrate()
	if err != nil {
		return nil, fmt.Errorf("failed to run developer migrations: %w", err)
	}
	logger.Info("developer database migrations completed")

	return &PostgresBackend{pool: pool}, nil
}

type DeveloperMigrationManager struct {
	pool *pgxpool.Pool
}

func (m *DeveloperMigrationManager) Migrate() error {
	goose.SetBaseFS(developerMigrations)
	defer goose.SetBaseFS(nil)

	err := goose.SetDialect("postgres")
	if err != nil {
		return fmt.Errorf("failed to set goose dialect: %w", err)
	}

	db := stdlib.OpenDBFromPool(m.pool)
	defer db.Close()

	err = goose.Up(db, "migrations/developer", goose.WithAllowMissing())
	if err != nil {
		return fmt.Errorf("failed to run developer migrations: %w", err)
	}

	return nil
}
