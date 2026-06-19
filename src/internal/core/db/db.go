package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/alecdray/two-cents/src/internal/core/db/sqlc"

	"github.com/pressly/goose/v3"

	_ "github.com/mattn/go-sqlite3"
)

const migrationsDir = "db/migrations"

// DB wraps the SQLite connection together with the sqlc-generated query set.
// Inside a transaction the query set is rebound to the *sql.Tx so reads and
// writes share the transaction's view (see WithTx).
type DB struct {
	sql     *sql.DB
	queries *sqlc.Queries
}

func NewDB(filepath string) (*DB, error) {
	// A busy timeout lets a write wait for a held lock to clear instead of
	// failing immediately with "database is locked" — the running app, the cron
	// sync, and the out-of-band set-password command all open the same file.
	sqlDb, err := sql.Open("sqlite3", filepath+"?_busy_timeout=5000")
	if err != nil {
		return nil, err
	}

	if err := sqlDb.Ping(); err != nil {
		return nil, err
	}

	sqlDb.SetMaxOpenConns(25)
	sqlDb.SetMaxIdleConns(5)
	sqlDb.SetConnMaxLifetime(5 * time.Minute)

	db := &DB{sql: sqlDb, queries: sqlc.New(sqlDb)}

	if err := db.runMigrations(); err != nil {
		return nil, err
	}

	return db, nil
}

// WrapSqlDB builds a *DB around an already-open *sql.DB without running
// migrations. Intended for tests that manage their own migration lifecycle.
func WrapSqlDB(sqlDB *sql.DB) *DB {
	return &DB{sql: sqlDB, queries: sqlc.New(sqlDB)}
}

// newDBWithTx returns a shallow copy of db whose query set is bound to tx, so
// every query issued through it participates in the transaction.
func newDBWithTx(db DB, tx *sql.Tx) *DB {
	db.queries = sqlc.New(tx)
	return &db
}

func (db *DB) Sql() *sql.DB {
	return db.sql
}

func (db *DB) Queries() *sqlc.Queries {
	return db.queries
}

func (db *DB) Close() error {
	return db.sql.Close()
}

// WithTx runs fn inside a transaction against a tx-bound *DB, committing on
// success and rolling back on error.
func (db *DB) WithTx(fn func(*DB) error) (err error) {
	tx, err := db.sql.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if err != nil {
			tx.Rollback()
		} else {
			err = tx.Commit()
		}
	}()

	err = fn(newDBWithTx(*db, tx))
	return err
}

func (db *DB) runMigrations() error {
	if err := goose.SetDialect("sqlite3"); err != nil {
		return err
	}

	err := goose.Up(db.sql, migrationsDir)
	if errors.Is(err, goose.ErrNoMigrationFiles) {
		return nil
	}
	return err
}
