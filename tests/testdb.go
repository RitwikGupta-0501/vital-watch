package tests

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/RitwikGupta-0501/vital-watch/internal/repository"
)

var (
	migrateOnce sync.Once
	migrateErr  error
)

// SetupTestDB connects to live PostgreSQL specified by DATABASE_URL (or default test DSN).
// If the database is unreachable, it cleanly skips the test with t.Skip.
func SetupTestDB(t *testing.T) (*pgxpool.Pool, *repository.DBRepository, func()) {
	t.Helper()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:password@localhost:5432/vitalwatch_test?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	config, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		t.Skipf("Skipping integration test: invalid DATABASE_URL %q: %v", dbURL, err)
		return nil, nil, nil
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Skipf("Skipping integration test: failed to initialize pgxpool: %v", err)
		return nil, nil, nil
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("Skipping integration test: PostgreSQL unreachable at %s: %v", dbURL, err)
		return nil, nil, nil
	}

	// Run migrations once across the test suite
	migrateOnce.Do(func() {
		migrateErr = runMigrations(pool)
	})

	if migrateErr != nil {
		pool.Close()
		t.Fatalf("Database migrations failed: %v", migrateErr)
	}

	repo := repository.New(pool, nil)

	t.Cleanup(func() {
		pool.Close()
	})

	teardown := func() {
		// Truncate tables between tests for isolation
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanCancel()
		_, _ = pool.Exec(cleanCtx, `
			TRUNCATE TABLE refresh_tokens, appointments, prescription_items, prescriptions, 
			               patient_vitals, patient_medication_logs, doctor_schedules, 
			               admin_profiles, doctor_profiles, patient_profiles, users CASCADE;
		`)
	}

	return pool, repo, teardown
}

func runMigrations(pool *pgxpool.Pool) error {
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()

	driver, err := pgxmigrate.WithInstance(sqlDB, &pgxmigrate.Config{})
	if err != nil {
		return fmt.Errorf("failed to create migration driver: %w", err)
	}

	// Locate migration directory relative to test package
	migrationsPath := "file://../migrations"
	if _, err := os.Stat("../migrations"); os.IsNotExist(err) {
		if _, err := os.Stat("migrations"); err == nil {
			migrationsPath = "file://migrations"
		}
	}

	m, err := migrate.NewWithDatabaseInstance(migrationsPath, "pgx5", driver)
	if err != nil {
		return fmt.Errorf("failed to initialize migrate: %w", err)
	}

	err = m.Up()
	_, _ = m.Close()
	if err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migration up failed: %w", err)
	}

	return nil
}
