package migrate

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"go.uber.org/zap"

	_ "github.com/lib/pq"
)

//go:embed all:migrations
var migrationsFS embed.FS

// AutoMigrate runs embedded migrations (from the migrations directory in the repository root)
// against the provided Postgres dbURL. It uses golang-migrate's iofs source to read the
// embedded migration files and database/postgres driver (via database/sql).
//
// Returns nil on success or if there are no changes (migrate.ErrNoChange treated as success).
func AutoMigrate(dbURL string, logger *zap.Logger) error {
	if dbURL == "" {
		return fmt.Errorf("dbURL is empty")
	}

	sqlDB, err := sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("failed to open database/sql DB: %w", err)
	}
	defer sqlDB.Close()

	if err := sqlDB.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	srcDriver, err := iofs.New(migrationsFS, "./migrations")
	if err != nil {
		return fmt.Errorf("failed to create iofs source driver: %w", err)
	}

	dbDriver, err := postgres.WithInstance(sqlDB, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("failed to create postgres driver instance: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", srcDriver, "postgres", dbDriver)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrations failed: %w", err)
	}

	if logger != nil {
		logger.Info("embedded database migrations applied (or up-to-date)")
	}
	return nil
}
