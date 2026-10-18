//go:build !js

package appsetup

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // the "sqlite" driver, in pure Go

	"github.com/arnodel/whymark"
	"github.com/arnodel/whymark/browser"
)

// openUserData opens the user's data in dir: tables of their own in the
// same database as the engine's pages, whymark.db.
func openUserData(dir string) (browser.UserData, error) {
	path := filepath.Join(dir, "whymark.db")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS user_latest_pages (
			document   TEXT PRIMARY KEY,
			page_id    TEXT NOT NULL,
			updated_at TEXT NOT NULL -- ISO 8601, in UTC
		);
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("making %s: %w", path, err)
	}
	return &sqliteUserData{db: db}, nil
}

// sqliteUserData is browser.UserData in a SQLite database.
type sqliteUserData struct {
	db *sql.DB
}

func (u *sqliteUserData) LatestPage(document whymark.Document) (string, error) {
	var id string
	err := u.db.QueryRow(`SELECT page_id FROM user_latest_pages WHERE document = ?`, string(document)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

func (u *sqliteUserData) SetLatestPage(document whymark.Document, id string) error {
	_, err := u.db.Exec(`INSERT INTO user_latest_pages (document, page_id, updated_at) VALUES (?, ?, ?)
		ON CONFLICT (document) DO UPDATE SET page_id = excluded.page_id, updated_at = excluded.updated_at`,
		string(document), id, time.Now().UTC().Format("2006-01-02T15:04:05.000000000Z"))
	return err
}
