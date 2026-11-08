// Package store 封装 SQLite 访问。WAL + 单写连接，读用连接池。
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// ErrNotFound 表示没有匹配行。
var ErrNotFound = errors.New("not found")

// Store 持有数据库句柄。
type Store struct {
	db   *sql.DB
	path string
}

// Open 打开库、套用 PRAGMA、建表并跑迁移。
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// SQLite 单写；连接数压到 1 避免 BUSY。
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	s := &Store{db: db, path: path}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, p := range []string{
		"journal_mode=WAL",
		"busy_timeout=8000",
		"synchronous=NORMAL",
		"foreign_keys=ON",
	} {
		if _, err := db.ExecContext(ctx, "PRAGMA "+p); err != nil {
			db.Close()
			return nil, fmt.Errorf("pragma %s: %w", p, err)
		}
	}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close 关闭库。
func (s *Store) Close() error { return s.db.Close() }

// DB 暴露底层句柄给上层测试用。
func (s *Store) DB() *sql.DB { return s.db }

const schemaVersion = 2

func (s *Store) migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS meta (
			key TEXT PRIMARY KEY,
			val TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS settings (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			api_key TEXT NOT NULL DEFAULT '',
			proxy TEXT NOT NULL DEFAULT '',
			domain TEXT NOT NULL DEFAULT 'icloud.com',
			region TEXT NOT NULL DEFAULT 'US',
			default_jobs INTEGER NOT NULL DEFAULT 1,
			default_wait INTEGER NOT NULL DEFAULT 5,
			add_password INTEGER NOT NULL DEFAULT 1,
			add_mfa INTEGER NOT NULL DEFAULT 1,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS accounts (
			id INTEGER PRIMARY KEY,
			email TEXT NOT NULL UNIQUE,
			password TEXT NOT NULL DEFAULT '',
			totp_secret TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL DEFAULT '',
			birthdate TEXT NOT NULL DEFAULT '',
			token TEXT NOT NULL DEFAULT '',
			proxy TEXT NOT NULL DEFAULT '',
			ip TEXT NOT NULL DEFAULT '',
			has_password INTEGER NOT NULL DEFAULT 0,
			mfa_state TEXT NOT NULL DEFAULT 'none',
			attempts INTEGER NOT NULL DEFAULT 0,
			error TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_kind ON accounts(has_password, totp_secret, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_mfa_state ON accounts(mfa_state)`,
		`CREATE TABLE IF NOT EXISTS mfa_events (
			id INTEGER PRIMARY KEY,
			account_id INTEGER NOT NULL,
			q INTEGER NOT NULL,
			event TEXT NOT NULL,
			note TEXT NOT NULL DEFAULT '',
			at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS run_items (
			id INTEGER PRIMARY KEY,
			run_id INTEGER NOT NULL,
			email TEXT NOT NULL DEFAULT '',
			stage TEXT NOT NULL DEFAULT '',
			state TEXT NOT NULL DEFAULT 'running',
			note TEXT NOT NULL DEFAULT '',
			at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS runs (
			id INTEGER PRIMARY KEY,
			started_at INTEGER NOT NULL,
			ended_at INTEGER NOT NULL DEFAULT 0,
			total INTEGER NOT NULL DEFAULT 0,
			ok INTEGER NOT NULL DEFAULT 0,
			fail INTEGER NOT NULL DEFAULT 0
		)`,
	}
	for _, q := range stmts {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("schema: %w", err)
		}
	}
	var cur int
	err := s.db.QueryRowContext(ctx, `SELECT CAST(val AS INTEGER) FROM meta WHERE key='schema'`).Scan(&cur)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = s.db.ExecContext(ctx,
			`INSERT INTO meta(key,val) VALUES('schema',?)`, schemaVersion)
		return err
	}
	if err != nil {
		return err
	}
	if cur < schemaVersion {
		// v2：设置表补 region 列（旧库没有）
		var has int
		_ = s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM pragma_table_info('settings') WHERE name='region'`).Scan(&has)
		if has == 0 {
			if _, err := s.db.ExecContext(ctx,
				`ALTER TABLE settings ADD COLUMN region TEXT NOT NULL DEFAULT 'US'`); err != nil {
				return fmt.Errorf("migrate v2: %w", err)
			}
		}
		_, err = s.db.ExecContext(ctx,
			`UPDATE meta SET val=? WHERE key='schema'`, schemaVersion)
	}
	return err
}

// SchemaVersion 供测试核对迁移结果。
func (s *Store) SchemaVersion(ctx context.Context) (int, error) {
	var v int
	err := s.db.QueryRowContext(ctx,
		`SELECT CAST(val AS INTEGER) FROM meta WHERE key='schema'`).Scan(&v)
	return v, err
}

// now 统一时间戳，便于测试替换。
var now = func() int64 { return time.Now().Unix() }
