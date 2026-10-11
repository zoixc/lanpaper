// SPDX-License-Identifier: MIT

package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// SessionRow is one persisted browser session. Digest is the hex SHA-256 of the
// bearer token; the token itself is never stored.
type SessionRow struct {
	Digest     string
	ID         string
	CreatedAt  int64
	Expires    int64
	Credential string
}

const sessionsImportedKey = "sessions_imported"

// ReplaceSessions stores the complete live session set in one transaction and
// marks the one-time sessions.json import as done, so the legacy file can never
// resurrect sessions that were revoked later.
func (s *SQLiteWallpaperStore) ReplaceSessions(ctx context.Context, rows []SessionRow) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := replaceSessionRows(ctx, tx, rows); err != nil {
		return err
	}
	return tx.Commit()
}

func replaceSessionRows(ctx context.Context, tx *sql.Tx, rows []SessionRow) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions`); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO sessions(digest,id,created_at,expires,credential) VALUES(?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, r := range rows {
		if _, err := stmt.ExecContext(ctx, r.Digest, r.ID, r.CreatedAt, r.Expires, r.Credential); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES(?,'1') ON CONFLICT(key) DO UPDATE SET value='1'`, sessionsImportedKey)
	return err
}

// LoadSessions returns every persisted session, including expired ones; the
// caller applies expiry and credential rules.
func (s *SQLiteWallpaperStore) LoadSessions(ctx context.Context) ([]SessionRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT digest,id,created_at,expires,credential FROM sessions ORDER BY digest`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SessionRow
	for rows.Next() {
		var r SessionRow
		if err := rows.Scan(&r.Digest, &r.ID, &r.CreatedAt, &r.Expires, &r.Credential); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SessionsImported reports whether the one-time sessions.json import has run.
func (s *SQLiteWallpaperStore) SessionsImported(ctx context.Context) (bool, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key=?`, sessionsImportedKey).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read session import marker: %w", err)
	}
	return v == "1", nil
}
