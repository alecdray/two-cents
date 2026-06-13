package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/pressly/goose/v3"

	_ "github.com/mattn/go-sqlite3"
)

const migrationsDir = "db/migrations"

// DB wraps the SQLite connection and runs goose migrations on open.
//
// sqlc-generated query code is NOT wired in yet — it lands with the first
// domain module's queries (see docs/architecture and sqlc.yaml). At that point
// DB gains a *sqlc.Queries and WithTx switches to handing back a tx-bound *DB,
// matching the wax pattern.
type DB struct {
	sql *sql.DB
}

func NewDB(filepath string) (*DB, error) {
	sqlDb, err := sql.Open("sqlite3", filepath)
	if err != nil {
		return nil, err
	}

	if err := sqlDb.Ping(); err != nil {
		return nil, err
	}

	sqlDb.SetMaxOpenConns(25)
	sqlDb.SetMaxIdleConns(5)
	sqlDb.SetConnMaxLifetime(5 * time.Minute)

	db := &DB{sql: sqlDb}

	if err := db.runMigrations(); err != nil {
		return nil, err
	}

	return db, nil
}

func (db *DB) Sql() *sql.DB {
	return db.sql
}

func (db *DB) Close() error {
	return db.sql.Close()
}

// WithTx runs fn inside a transaction, committing on success and rolling back
// on error.
func (db *DB) WithTx(fn func(tx *sql.Tx) error) (err error) {
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

	err = fn(tx)
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
