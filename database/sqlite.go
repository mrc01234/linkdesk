package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mattn/go-sqlite3"
	"linkdesk/model"
)

type Database struct{ db *sql.DB }

func New(dbPath string) (*Database, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}
	// A small single-instance tool: serialize local access, including :memory:.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	d := &Database{db: db}
	if _, err = db.Exec("PRAGMA busy_timeout = 5000; PRAGMA journal_mode = WAL;"); err == nil {
		err = d.initSchema()
	}
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize database: %w", err)
	}
	return d, nil
}

func (d *Database) Close() error { return d.db.Close() }

func (d *Database) initSchema() error {
	_, err := d.db.Exec(`
        CREATE TABLE IF NOT EXISTS urls (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            short_code TEXT UNIQUE NOT NULL,
            long_url TEXT NOT NULL,
            created_at TIMESTAMP NOT NULL,
            clicks INTEGER NOT NULL DEFAULT 0,
            expires_at INTEGER
        );
        CREATE INDEX IF NOT EXISTS idx_urls_created_id ON urls(created_at DESC, id DESC);
    `)
	if err != nil {
		return err
	}
	// Upgrade the upstream schema without losing links. Unix seconds avoid
	// ambiguous timestamp/string comparisons for expiry across time zones.
	rows, err := d.db.Query("PRAGMA table_info(urls)")
	if err != nil {
		return err
	}
	hasExpiry := false
	for rows.Next() {
		var cid, notNull, pk int
		var name, kind string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &pk); err != nil {
			rows.Close()
			return err
		}
		hasExpiry = hasExpiry || name == "expires_at"
	}
	err = rows.Err()
	rows.Close() // release the single connection before ALTER
	if err != nil {
		return err
	}
	if !hasExpiry {
		_, err = d.db.Exec("ALTER TABLE urls ADD COLUMN expires_at INTEGER")
	}
	return err
}

func (d *Database) SaveURL(u *model.URL) error {
	var expiry any
	if u.ExpiresAt != nil {
		expiry = u.ExpiresAt.Unix()
	}
	result, err := d.db.Exec(
		"INSERT INTO urls (short_code, long_url, created_at, clicks, expires_at) VALUES (?, ?, ?, ?, ?)",
		u.ShortCode, u.LongURL, u.CreatedAt.UTC(), u.Clicks, expiry)
	if err != nil {
		var sqliteErr sqlite3.Error
		if errors.As(err, &sqliteErr) && sqliteErr.ExtendedCode == sqlite3.ErrConstraintUnique {
			return &model.ErrCustomCodeAlreadyExists{Code: u.ShortCode}
		}
		return fmt.Errorf("save URL: %w", err)
	}
	u.ID, err = result.LastInsertId()
	return err
}

type scanner interface{ Scan(...any) error }

func scanURL(row scanner) (*model.URL, error) {
	var u model.URL
	var expiry sql.NullInt64
	if err := row.Scan(&u.ID, &u.ShortCode, &u.LongURL, &u.CreatedAt, &u.Clicks, &expiry); err != nil {
		return nil, err
	}
	if expiry.Valid {
		t := time.Unix(expiry.Int64, 0).UTC()
		u.ExpiresAt = &t
	}
	return &u, nil
}

const columns = "id, short_code, long_url, created_at, clicks, expires_at"

func (d *Database) GetURLByShortCode(code string) (*model.URL, error) {
	u, err := scanURL(d.db.QueryRow("SELECT "+columns+" FROM urls WHERE short_code = ?", code))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return u, err
}

func (d *Database) IncrementClicks(code string) error {
	result, err := d.db.Exec("UPDATE urls SET clicks = clicks + 1 WHERE short_code = ?", code)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return &model.ErrURLNotFound{Code: code}
	}
	return nil
}

// ListURLs is retained for the upstream CLI. Web/API callers use SearchURLs.
func (d *Database) ListURLs() ([]*model.URL, error) {
	rows, err := d.db.Query("SELECT " + columns + " FROM urls ORDER BY created_at DESC, id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return readURLs(rows)
}

func readURLs(rows *sql.Rows) ([]*model.URL, error) {
	urls := make([]*model.URL, 0)
	for rows.Next() {
		u, err := scanURL(rows)
		if err != nil {
			return nil, err
		}
		urls = append(urls, u)
	}
	return urls, rows.Err()
}

func (d *Database) SearchURLs(ctx context.Context, opt model.ListOptions) (*model.URLPage, error) {
	if opt.Page < 1 || opt.Page > 1000000 || opt.PageSize < 1 || opt.PageSize > 100 {
		return nil, &model.ErrInvalidInput{Message: "invalid pagination"}
	}
	where := []string{"1=1"}
	args := []any{}
	if opt.Query != "" {
		// Literal substring search: % and _ in user input are not wildcards.
		where = append(where, "(instr(lower(short_code), lower(?)) > 0 OR instr(lower(long_url), lower(?)) > 0)")
		args = append(args, opt.Query, opt.Query)
	}
	now := time.Now().Unix()
	switch opt.Status {
	case "", "all":
	case "active":
		where = append(where, "(expires_at IS NULL OR expires_at > ?)")
		args = append(args, now)
	case "expired":
		where = append(where, "(expires_at IS NOT NULL AND expires_at <= ?)")
		args = append(args, now)
	default:
		return nil, &model.ErrInvalidInput{Message: "status must be all, active or expired"}
	}
	filter := " WHERE " + strings.Join(where, " AND ")
	// Both count and items observe one read transaction/snapshot.
	tx, err := d.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	page := &model.URLPage{URLs: make([]*model.URL, 0), Page: opt.Page, PageSize: opt.PageSize}
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM urls"+filter, args...).Scan(&page.Total); err != nil {
		return nil, err
	}
	page.TotalPages = (page.Total + opt.PageSize - 1) / opt.PageSize
	args = append(args, opt.PageSize, (opt.Page-1)*opt.PageSize)
	rows, err := tx.QueryContext(ctx, "SELECT "+columns+" FROM urls"+filter+" ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?", args...)
	if err != nil {
		return nil, err
	}
	page.URLs, err = readURLs(rows)
	closeErr := rows.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return page, nil
}

func (d *Database) DeleteURL(code string) error {
	_, err := d.db.Exec("DELETE FROM urls WHERE short_code = ?", code)
	return err
}
