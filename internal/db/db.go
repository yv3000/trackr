// Package db wraps the trackr SQLite database stored at
// %USERPROFILE%\.trackr\trackr.db. It uses the pure-Go modernc.org/sqlite
// driver so no external DLL is required.
package db

import (
	"database/sql"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB is a thin handle around the underlying *sql.DB connection.
type DB struct {
	conn *sql.DB
}

// Install is one logged install-history row.
type Install struct {
	ID          int64
	Name        string
	Tool        string
	Command     string
	InstallDir  string
	RegistryKey string
	WhyTag      string
	InstalledAt string
}

// Path returns the absolute path to the trackr database file, creating the
// parent directory if necessary.
func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".trackr")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "trackr.db"), nil
}

// Open opens (and migrates) the trackr database.
func Open() (*DB, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	conn, err := sql.Open("sqlite", p)
	if err != nil {
		return nil, err
	}
	d := &DB{conn: conn}
	if err := d.migrate(); err != nil {
		conn.Close()
		return nil, err
	}
	return d, nil
}

// Close releases the database connection.
func (d *DB) Close() error { return d.conn.Close() }

func (d *DB) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS installs (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    name         TEXT NOT NULL,
    tool         TEXT NOT NULL,
    command      TEXT NOT NULL,
    install_dir  TEXT,
    registry_key TEXT,
    why_tag      TEXT DEFAULT '',
    installed_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS snapshots (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    install_id    INTEGER REFERENCES installs(id),
    files_json    TEXT,
    reg_keys_json TEXT,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);`
	_, err := d.conn.Exec(schema)
	return err
}

// AddInstall records a new install-history row and returns its id.
func (d *DB) AddInstall(in Install) (int64, error) {
	res, err := d.conn.Exec(
		`INSERT INTO installs (name, tool, command, install_dir, registry_key, why_tag)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		in.Name, in.Tool, in.Command, in.InstallDir, in.RegistryKey, in.WhyTag,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListInstalls returns all logged installs, most recent first.
func (d *DB) ListInstalls() ([]Install, error) {
	rows, err := d.conn.Query(
		`SELECT id, name, tool, command,
		        COALESCE(install_dir, ''), COALESCE(registry_key, ''),
		        COALESCE(why_tag, ''), installed_at
		 FROM installs ORDER BY installed_at DESC, id DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Install
	for rows.Next() {
		var in Install
		if err := rows.Scan(&in.ID, &in.Name, &in.Tool, &in.Command,
			&in.InstallDir, &in.RegistryKey, &in.WhyTag, &in.InstalledAt); err != nil {
			return nil, err
		}
		out = append(out, in)
	}
	return out, rows.Err()
}

// AddSnapshot stores the file/registry footprint captured for an install.
func (d *DB) AddSnapshot(installID int64, filesJSON, regKeysJSON string) error {
	_, err := d.conn.Exec(
		`INSERT INTO snapshots (install_id, files_json, reg_keys_json) VALUES (?, ?, ?)`,
		installID, filesJSON, regKeysJSON,
	)
	return err
}
