package store

import (
	"database/sql"
	"encoding/json"
	"sort"
	"strings"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	_ "modernc.org/sqlite" // pure-Go SQLite driver (no CGo), registers "sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS servers (
  id          TEXT PRIMARY KEY,
  name        TEXT    NOT NULL,
  type        INTEGER NOT NULL,
  mc_version  TEXT    NOT NULL,
  memory_mb   INTEGER NOT NULL,
  port        INTEGER NOT NULL,
  status      INTEGER NOT NULL,
  sort_order  INTEGER NOT NULL DEFAULT 0,
  java_major  INTEGER NOT NULL,
  launch_args TEXT    NOT NULL,
  custom_java_args TEXT NOT NULL DEFAULT ''
);`

// SQLite is a Store persisted in a SQLite database file, so the registry
// survives engine restarts (PROMPT §10.5).
type SQLite struct {
	db *sql.DB
}

// OpenSQLite opens (creating if needed) the registry database at path.
func OpenSQLite(path string) (*SQLite, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// SQLite is single-writer; cap connections to avoid "database is locked".
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureServerColumns(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &SQLite{db: db}, nil
}

func (s *SQLite) Close() error { return s.db.Close() }

func (s *SQLite) Put(srv *Server) error {
	args, err := json.Marshal(srv.LaunchArgs)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		INSERT INTO servers (id, name, type, mc_version, memory_mb, port, status, sort_order, java_major, launch_args, custom_java_args)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name, type=excluded.type, mc_version=excluded.mc_version,
			memory_mb=excluded.memory_mb, port=excluded.port, status=excluded.status,
			sort_order=excluded.sort_order, java_major=excluded.java_major,
			launch_args=excluded.launch_args, custom_java_args=excluded.custom_java_args`,
		srv.ID, srv.Name, int(srv.Type), srv.McVersion, srv.MemoryMB, srv.Port,
		int(srv.Status), srv.SortOrder, srv.JavaMajor, string(args), srv.CustomJavaArgs)
	return err
}

func (s *SQLite) Get(id string) (*Server, bool) {
	row := s.db.QueryRow(`
		SELECT id, name, type, mc_version, memory_mb, port, status, sort_order, java_major, launch_args, custom_java_args
		FROM servers WHERE id = ?`, id)
	srv, err := scanServer(row)
	if err != nil {
		return nil, false
	}
	return srv, true
}

func (s *SQLite) List() []*Server {
	rows, err := s.db.Query(`
		SELECT id, name, type, mc_version, memory_mb, port, status, sort_order, java_major, launch_args, custom_java_args
		FROM servers`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []*Server
	for rows.Next() {
		if srv, err := scanServer(rows); err == nil {
			out = append(out, srv)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func (s *SQLite) Delete(id string) error {
	_, err := s.db.Exec(`DELETE FROM servers WHERE id = ?`, id)
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanServer(sc rowScanner) (*Server, error) {
	var (
		srv      Server
		typ      int
		status   int
		argsJSON string
	)
	if err := sc.Scan(&srv.ID, &srv.Name, &typ, &srv.McVersion, &srv.MemoryMB,
		&srv.Port, &status, &srv.SortOrder, &srv.JavaMajor, &argsJSON, &srv.CustomJavaArgs); err != nil {
		return nil, err
	}
	srv.Type = mcmanagerv1.ServerType(typ)
	srv.Status = mcmanagerv1.ServerStatus(status)
	if argsJSON != "" {
		_ = json.Unmarshal([]byte(argsJSON), &srv.LaunchArgs)
	}
	return &srv, nil
}

func ensureServerColumns(db *sql.DB) error {
	if _, err := db.Exec(`ALTER TABLE servers ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0`); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return err
		}
	}
	if _, err := db.Exec(`ALTER TABLE servers ADD COLUMN custom_java_args TEXT NOT NULL DEFAULT ''`); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return err
		}
	}
	return nil
}
