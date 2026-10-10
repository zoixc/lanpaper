// SPDX-License-Identifier: MIT

package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/ncruces/go-sqlite3/driver"
)

const sqliteSchema = `
PRAGMA foreign_keys = ON;
CREATE TABLE IF NOT EXISTS wallpapers (
  link_name TEXT PRIMARY KEY CHECK(length(link_name) BETWEEN 1 AND 64),
  category TEXT NOT NULL DEFAULT 'other',
  image_url TEXT NOT NULL DEFAULT '', preview TEXT NOT NULL DEFAULT '',
  has_image INTEGER NOT NULL DEFAULT 0 CHECK(has_image IN (0,1)),
  mime_type TEXT NOT NULL DEFAULT '', size_bytes INTEGER NOT NULL DEFAULT 0 CHECK(size_bytes >= 0),
  mod_time INTEGER NOT NULL DEFAULT 0, created_at INTEGER NOT NULL DEFAULT 0,
  is_pinned INTEGER NOT NULL DEFAULT 0 CHECK(is_pinned IN (0,1)), pinned_at INTEGER NOT NULL DEFAULT 0,
  access_level TEXT NOT NULL DEFAULT 'public' CHECK(access_level IN ('public','local','token','auth')),
  access_token TEXT NOT NULL DEFAULT '', current_version INTEGER NOT NULL DEFAULT 0 CHECK(current_version >= 0)
) STRICT;
CREATE TABLE IF NOT EXISTS history (
  link_name TEXT NOT NULL REFERENCES wallpapers(link_name) ON DELETE CASCADE ON UPDATE CASCADE,
  version INTEGER NOT NULL CHECK(version > 0), ext TEXT NOT NULL, size_bytes INTEGER NOT NULL CHECK(size_bytes >= 0),
  mtime INTEGER NOT NULL DEFAULT 0, saved_at INTEGER NOT NULL DEFAULT 0, PRIMARY KEY(link_name, version)
) STRICT;
CREATE TABLE IF NOT EXISTS playlist_items (
  link_name TEXT NOT NULL REFERENCES wallpapers(link_name) ON DELETE CASCADE ON UPDATE CASCADE,
  item_id INTEGER NOT NULL CHECK(item_id > 0), ext TEXT NOT NULL, size_bytes INTEGER NOT NULL CHECK(size_bytes >= 0),
  added_at INTEGER NOT NULL DEFAULT 0, mod_time INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY(link_name, item_id)
) STRICT;
CREATE TABLE IF NOT EXISTS rotations (
  link_name TEXT PRIMARY KEY REFERENCES wallpapers(link_name) ON DELETE CASCADE ON UPDATE CASCADE,
  enabled INTEGER NOT NULL CHECK(enabled IN (0,1)), interval_seconds INTEGER NOT NULL CHECK(interval_seconds >= 5),
  ordering TEXT NOT NULL CHECK(ordering IN ('sequential','random'))
) STRICT;
CREATE INDEX IF NOT EXISTS wallpapers_created_idx ON wallpapers(created_at DESC, link_name);
CREATE INDEX IF NOT EXISTS wallpapers_access_idx ON wallpapers(access_level, created_at DESC);
`

// SQLiteWallpaperStore implements WallpaperStore with normalized metadata.
// Media bytes remain in the existing filesystem layout.
type SQLiteWallpaperStore struct {
	db   *sql.DB
	path string
}

func OpenSQLiteWallpaperStore(ctx context.Context, path string) (*SQLiteWallpaperStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	dsn := "file:" + filepath.ToSlash(path) + "?_txlock=immediate&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &SQLiteWallpaperStore{db: db, path: path}
	if _, err = db.ExecContext(ctx, "PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL;"+sqliteSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize sqlite: %w", err)
	}
	return store, nil
}

func (s *SQLiteWallpaperStore) Close() error { return s.db.Close() }
func (s *SQLiteWallpaperStore) Path() string { return s.path }

type sqlQuerier interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

const wallpaperColumns = `link_name,category,image_url,preview,has_image,mime_type,size_bytes,mod_time,created_at,is_pinned,pinned_at,access_level,access_token,current_version`

func scanWallpaper(row interface{ Scan(...any) error }) (*Wallpaper, error) {
	wallpaper := &Wallpaper{}
	err := row.Scan(&wallpaper.LinkName, &wallpaper.Category, &wallpaper.ImageURL, &wallpaper.Preview,
		&wallpaper.HasImage, &wallpaper.MIMEType, &wallpaper.SizeBytes, &wallpaper.ModTime, &wallpaper.CreatedAt,
		&wallpaper.IsPinned, &wallpaper.PinnedAt, &wallpaper.AccessLevel, &wallpaper.AccessToken, &wallpaper.CurrentVersion)
	if err != nil {
		return nil, err
	}
	wallpaper.ID = wallpaper.LinkName
	deriveWallpaperPaths(wallpaper)
	return wallpaper, nil
}

func deriveWallpaperPaths(wallpaper *Wallpaper) {
	if !wallpaper.HasImage || wallpaper.MIMEType == "" {
		return
	}
	wallpaper.ImagePath = MediaPath(wallpaper.LinkName, wallpaper.MIMEType)
	if wallpaper.MIMEType != "mp4" && wallpaper.MIMEType != "webm" {
		wallpaper.PreviewPath = PreviewFilePath(wallpaper.LinkName)
	}
}

func loadRelations(ctx context.Context, query sqlQuerier, wallpaper *Wallpaper) error {
	rows, err := query.QueryContext(ctx, `SELECT version,ext,size_bytes,mtime,saved_at FROM history WHERE link_name=? ORDER BY version DESC`, wallpaper.LinkName)
	if err != nil {
		return err
	}
	for rows.Next() {
		var entry HistoryEntry
		if err := rows.Scan(&entry.Version, &entry.Ext, &entry.SizeBytes, &entry.ModTime, &entry.SavedAt); err != nil {
			rows.Close()
			return err
		}
		wallpaper.History = append(wallpaper.History, entry)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	rows, err = query.QueryContext(ctx, `SELECT item_id,ext,size_bytes,added_at,mod_time FROM playlist_items WHERE link_name=? ORDER BY item_id`, wallpaper.LinkName)
	if err != nil {
		return err
	}
	for rows.Next() {
		var item PlaylistItem
		if err := rows.Scan(&item.ID, &item.Ext, &item.SizeBytes, &item.AddedAt, &item.ModTime); err != nil {
			rows.Close()
			return err
		}
		wallpaper.Items = append(wallpaper.Items, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	var rotation RotateConfig
	err = query.QueryRowContext(ctx, `SELECT enabled,interval_seconds,ordering FROM rotations WHERE link_name=?`, wallpaper.LinkName).
		Scan(&rotation.Enabled, &rotation.Interval, &rotation.Order)
	if err == nil {
		wallpaper.Rotate = &rotation
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return nil
}

func getSQL(ctx context.Context, query sqlQuerier, id string) (*Wallpaper, bool, error) {
	wallpaper, err := scanWallpaper(query.QueryRowContext(ctx, `SELECT `+wallpaperColumns+` FROM wallpapers WHERE link_name=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if err := loadRelations(ctx, query, wallpaper); err != nil {
		return nil, false, err
	}
	return wallpaper, true, nil
}

func listSQL(ctx context.Context, query sqlQuerier) ([]*Wallpaper, error) {
	rows, err := query.QueryContext(ctx, `SELECT `+wallpaperColumns+` FROM wallpapers ORDER BY is_pinned DESC,pinned_at DESC,created_at DESC,link_name`)
	if err != nil {
		return nil, err
	}
	var list []*Wallpaper
	for rows.Next() {
		wallpaper, err := scanWallpaper(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		list = append(list, wallpaper)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for _, wallpaper := range list {
		if err := loadRelations(ctx, query, wallpaper); err != nil {
			return nil, err
		}
	}
	return list, nil
}

func (s *SQLiteWallpaperStore) Get(ctx context.Context, id string) (*Wallpaper, bool, error) {
	return getSQL(ctx, s.db, id)
}
func (s *SQLiteWallpaperStore) List(ctx context.Context) ([]*Wallpaper, error) {
	return listSQL(ctx, s.db)
}
func (s *SQLiteWallpaperStore) Create(ctx context.Context, wallpaper *Wallpaper) error {
	return s.Transact(ctx, func(tx WallpaperTransaction) error { return tx.Create(wallpaper) })
}
func (s *SQLiteWallpaperStore) Update(ctx context.Context, id string, edit func(*Wallpaper) error) (*Wallpaper, error) {
	var output *Wallpaper
	err := s.Transact(ctx, func(tx WallpaperTransaction) error { var err error; output, err = tx.Update(id, edit); return err })
	return output, err
}
func (s *SQLiteWallpaperStore) Rename(ctx context.Context, oldName, newName string) (*Wallpaper, error) {
	var output *Wallpaper
	err := s.Transact(ctx, func(tx WallpaperTransaction) error {
		var err error
		output, err = tx.Rename(oldName, newName)
		return err
	})
	return output, err
}
func (s *SQLiteWallpaperStore) Delete(ctx context.Context, id string) (*Wallpaper, error) {
	var output *Wallpaper
	err := s.Transact(ctx, func(tx WallpaperTransaction) error { var err error; output, err = tx.Delete(id); return err })
	return output, err
}

func writeSQL(ctx context.Context, tx *sql.Tx, wallpaper *Wallpaper) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO wallpapers (`+wallpaperColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(link_name) DO UPDATE SET category=excluded.category,image_url=excluded.image_url,preview=excluded.preview,
has_image=excluded.has_image,mime_type=excluded.mime_type,size_bytes=excluded.size_bytes,mod_time=excluded.mod_time,
created_at=excluded.created_at,is_pinned=excluded.is_pinned,pinned_at=excluded.pinned_at,access_level=excluded.access_level,
access_token=excluded.access_token,current_version=excluded.current_version`,
		wallpaper.LinkName, wallpaper.Category, wallpaper.ImageURL, wallpaper.Preview, wallpaper.HasImage, wallpaper.MIMEType,
		wallpaper.SizeBytes, wallpaper.ModTime, wallpaper.CreatedAt, wallpaper.IsPinned, wallpaper.PinnedAt,
		NormalizeAccessLevel(wallpaper.AccessLevel), wallpaper.AccessToken, wallpaper.CurrentVersion)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM history WHERE link_name=?`, wallpaper.LinkName); err != nil {
		return err
	}
	for _, entry := range wallpaper.History {
		if _, err = tx.ExecContext(ctx, `INSERT INTO history(link_name,version,ext,size_bytes,mtime,saved_at) VALUES(?,?,?,?,?,?)`, wallpaper.LinkName, entry.Version, entry.Ext, entry.SizeBytes, entry.ModTime, entry.SavedAt); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM playlist_items WHERE link_name=?`, wallpaper.LinkName); err != nil {
		return err
	}
	for _, item := range wallpaper.Items {
		if _, err = tx.ExecContext(ctx, `INSERT INTO playlist_items(link_name,item_id,ext,size_bytes,added_at,mod_time) VALUES(?,?,?,?,?,?)`, wallpaper.LinkName, item.ID, item.Ext, item.SizeBytes, item.AddedAt, item.ModTime); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM rotations WHERE link_name=?`, wallpaper.LinkName); err != nil {
		return err
	}
	if wallpaper.Rotate != nil {
		_, err = tx.ExecContext(ctx, `INSERT INTO rotations(link_name,enabled,interval_seconds,ordering) VALUES(?,?,?,?)`, wallpaper.LinkName, wallpaper.Rotate.Enabled, wallpaper.Rotate.Interval, wallpaper.Rotate.Order)
	}
	return err
}

type sqliteTransaction struct {
	ctx context.Context
	tx  *sql.Tx
}

func (tx *sqliteTransaction) Get(id string) (*Wallpaper, bool) {
	value, ok, _ := getSQL(tx.ctx, tx.tx, id)
	return value, ok
}
func (tx *sqliteTransaction) Create(wallpaper *Wallpaper) error {
	if _, exists := tx.Get(wallpaper.LinkName); exists {
		return ErrExists
	}
	return writeSQL(tx.ctx, tx.tx, cloneWallpaper(wallpaper))
}
func (tx *sqliteTransaction) Update(id string, edit func(*Wallpaper) error) (*Wallpaper, error) {
	wallpaper, exists, err := getSQL(tx.ctx, tx.tx, id)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	if err := edit(wallpaper); err != nil {
		return nil, err
	}
	if wallpaper.LinkName != id {
		return nil, errors.New("update cannot rename link")
	}
	if err := writeSQL(tx.ctx, tx.tx, wallpaper); err != nil {
		return nil, err
	}
	return cloneWallpaper(wallpaper), nil
}
func (tx *sqliteTransaction) Rename(oldName, newName string) (*Wallpaper, error) {
	wallpaper, exists, err := getSQL(tx.ctx, tx.tx, oldName)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	if _, exists := tx.Get(newName); exists {
		return nil, ErrExists
	}
	preview := wallpaper.Preview
	if preview != "" {
		preview = "/api/preview/" + newName
	}
	if _, err := tx.tx.ExecContext(tx.ctx, `UPDATE wallpapers SET link_name=?,image_url=?,preview=? WHERE link_name=?`, newName, "/"+newName, preview, oldName); err != nil {
		return nil, err
	}
	wallpaper.ID, wallpaper.LinkName, wallpaper.ImageURL, wallpaper.Preview = newName, newName, "/"+newName, preview
	deriveWallpaperPaths(wallpaper)
	return wallpaper, nil
}
func (tx *sqliteTransaction) Delete(id string) (*Wallpaper, error) {
	wallpaper, exists, err := getSQL(tx.ctx, tx.tx, id)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	if _, err := tx.tx.ExecContext(tx.ctx, `DELETE FROM wallpapers WHERE link_name=?`, id); err != nil {
		return nil, err
	}
	return wallpaper, nil
}
func (tx *sqliteTransaction) List() []*Wallpaper { list, _ := listSQL(tx.ctx, tx.tx); return list }

func (s *SQLiteWallpaperStore) Transact(ctx context.Context, apply func(WallpaperTransaction) error) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := apply(&sqliteTransaction{ctx: ctx, tx: tx}); err != nil {
		return err
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteWallpaperStore) IntegrityCheck(ctx context.Context) error {
	var result string
	if err := s.db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&result); err != nil {
		return err
	}
	if result != "ok" {
		return fmt.Errorf("sqlite integrity check: %s", result)
	}
	return nil
}

func (s *SQLiteWallpaperStore) Backup(ctx context.Context, destination string) error {
	if strings.TrimSpace(destination) == "" {
		return errors.New("backup destination is empty")
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	quoted := strings.ReplaceAll(destination, "'", "''")
	_, err := s.db.ExecContext(ctx, `VACUUM INTO '`+quoted+`'`)
	return err
}

var _ WallpaperStore = (*SQLiteWallpaperStore)(nil)
