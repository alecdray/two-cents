package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/alecdray/two-cents/src/internal/core/db/sqlc"

	"github.com/pressly/goose/v3"

	_ "github.com/mattn/go-sqlite3"
)

const migrationsDir = "db/migrations"

// Executor is the statement surface shared by *sql.DB and *sql.Tx. Sql()
// returns it rather than the pool handle so that raw SQL issued from a
// tx-bound *DB runs inside that transaction instead of beside it.
type Executor interface {
	Exec(query string, args ...any) (sql.Result, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// DB wraps the SQLite connection together with the sqlc-generated query set.
// Inside a transaction both are rebound to the *sql.Tx — the query set and the
// raw Executor handed out by Sql() — so every read and write issued through a
// tx-bound *DB shares the transaction's view (see WithTx).
type DB struct {
	sql *sql.DB
	// tx is non-nil only on the tx-bound copy WithTx passes to its callback.
	// It is the single marker of "inside a transaction": Sql() routes through
	// it, and WithTx joins it instead of opening a second transaction.
	tx      *sql.Tx
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

// newDBWithTx returns a shallow copy of db bound to tx — both its query set
// and the Executor behind Sql() — so every query issued through it
// participates in the transaction.
func newDBWithTx(db DB, tx *sql.Tx) *DB {
	db.tx = tx
	db.queries = sqlc.New(tx)
	return &db
}

// Sql exposes the raw statement surface for queries sqlc does not generate. On
// a tx-bound *DB it is the transaction itself, never the pool.
func (db *DB) Sql() Executor {
	if db.tx != nil {
		return db.tx
	}
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
//
// Called on a *DB that is already tx-bound it joins that transaction rather
// than opening a second one on the pool (which SQLite would block on): fn sees
// the same handle, and the outermost WithTx keeps sole ownership of the
// commit/rollback boundary.
func (db *DB) WithTx(fn func(*DB) error) (err error) {
	if db.tx != nil {
		return fn(db)
	}

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
