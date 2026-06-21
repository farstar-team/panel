package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type Tunnel struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Role          string   `json:"role"`
	Protocol      string   `json:"protocol"`
	ListenAddr    string   `json:"listen_addr"`
	RemoteAddr    string   `json:"remote_addr"`
	PublicPorts   []string `json:"public_ports"`
	LocalServices []string `json:"local_services"`
	Secret        string   `json:"secret,omitempty"`
	SecretCipher  string   `json:"-"`
	TLSCert       string   `json:"tls_cert"`
	TLSKey        string   `json:"tls_key"`
	TLSServerName string   `json:"tls_server_name"`
	TLSCACert     string   `json:"tls_ca_cert"`
	SkipTLSVerify bool     `json:"skip_tls_verify"`
	Autostart     bool     `json:"autostart"`
	Status        string   `json:"status"`
	PID           int      `json:"pid"`
	ActiveConns   int64    `json:"active_connections"`
	BytesIn       int64    `json:"bytes_in"`
	BytesOut      int64    `json:"bytes_out"`
	LastError     string   `json:"last_error"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
}

type Runtime struct {
	Status      string
	PID         int
	ActiveConns int64
	BytesIn     int64
	BytesOut    int64
	LastError   string
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	s := &Store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	const schema = `
CREATE TABLE IF NOT EXISTS users (
  id INTEGER PRIMARY KEY,
  username TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
  token_hash TEXT PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  csrf_token TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS tunnels (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  role TEXT NOT NULL,
  protocol TEXT NOT NULL,
  listen_addr TEXT NOT NULL DEFAULT '',
  remote_addr TEXT NOT NULL DEFAULT '',
  public_ports TEXT NOT NULL DEFAULT '[]',
  local_services TEXT NOT NULL DEFAULT '[]',
  secret_cipher TEXT NOT NULL,
  tls_cert TEXT NOT NULL DEFAULT '',
  tls_key TEXT NOT NULL DEFAULT '',
  tls_server_name TEXT NOT NULL DEFAULT '',
  tls_ca_cert TEXT NOT NULL DEFAULT '',
  skip_tls_verify INTEGER NOT NULL DEFAULT 0,
  autostart INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'stopped',
  pid INTEGER NOT NULL DEFAULT 0,
  active_connections INTEGER NOT NULL DEFAULT 0,
  bytes_in INTEGER NOT NULL DEFAULT 0,
  bytes_out INTEGER NOT NULL DEFAULT 0,
  last_error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_expiry ON sessions(expires_at);
CREATE INDEX IF NOT EXISTS idx_tunnels_autostart ON tunnels(autostart);
`
	_, err := s.db.ExecContext(ctx, schema)
	return err
}

func (s *Store) UserCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	return count, err
}

func (s *Store) CreateAdmin(ctx context.Context, username, passwordHash string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username is required")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users(username,password_hash,created_at) VALUES(?,?,?)`,
		username, passwordHash, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) UserByName(ctx context.Context, username string) (int64, string, error) {
	var id int64
	var hash string
	err := s.db.QueryRowContext(ctx,
		`SELECT id,password_hash FROM users WHERE username=?`, username).Scan(&id, &hash)
	return id, hash, err
}

func (s *Store) CreateSession(ctx context.Context, tokenHash string, userID int64, csrf string, expires time.Time) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO sessions(token_hash,user_id,csrf_token,expires_at,created_at)
VALUES(?,?,?,?,?)`,
		tokenHash, userID, csrf, expires.UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) Session(ctx context.Context, tokenHash string) (int64, string, error) {
	var userID int64
	var csrf, expiry string
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id,csrf_token,expires_at FROM sessions WHERE token_hash=?`,
		tokenHash).Scan(&userID, &csrf, &expiry)
	if err != nil {
		return 0, "", err
	}
	expires, err := time.Parse(time.RFC3339, expiry)
	if err != nil || time.Now().After(expires) {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash=?`, tokenHash)
		return 0, "", sql.ErrNoRows
	}
	return userID, csrf, nil
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash=?`, tokenHash)
	return err
}

func (s *Store) CleanupSessions(ctx context.Context) {
	_, _ = s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ?`, time.Now().UTC().Format(time.RFC3339))
}

func (s *Store) ListTunnels(ctx context.Context) ([]Tunnel, error) {
	rows, err := s.db.QueryContext(ctx, tunnelSelect+` ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Tunnel
	for rows.Next() {
		t, err := scanTunnel(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, rows.Err()
}

func (s *Store) AutostartTunnels(ctx context.Context) ([]Tunnel, error) {
	rows, err := s.db.QueryContext(ctx, tunnelSelect+` WHERE autostart=1 ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Tunnel
	for rows.Next() {
		t, err := scanTunnel(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, rows.Err()
}

func (s *Store) Tunnel(ctx context.Context, id string) (Tunnel, error) {
	return scanTunnel(s.db.QueryRowContext(ctx, tunnelSelect+` WHERE id=?`, id))
}

func (s *Store) CreateTunnel(ctx context.Context, t Tunnel) error {
	now := time.Now().UTC().Format(time.RFC3339)
	publicPorts, _ := json.Marshal(t.PublicPorts)
	localServices, _ := json.Marshal(t.LocalServices)
	_, err := s.db.ExecContext(ctx, `
INSERT INTO tunnels(
 id,name,role,protocol,listen_addr,remote_addr,public_ports,local_services,
 secret_cipher,tls_cert,tls_key,tls_server_name,tls_ca_cert,skip_tls_verify,
 autostart,status,created_at,updated_at
) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.ID, t.Name, t.Role, t.Protocol, t.ListenAddr, t.RemoteAddr, string(publicPorts),
		string(localServices), t.SecretCipher, t.TLSCert, t.TLSKey, t.TLSServerName,
		t.TLSCACert, boolInt(t.SkipTLSVerify), boolInt(t.Autostart), "stopped", now, now)
	return err
}

func (s *Store) UpdateTunnel(ctx context.Context, t Tunnel) error {
	publicPorts, _ := json.Marshal(t.PublicPorts)
	localServices, _ := json.Marshal(t.LocalServices)
	_, err := s.db.ExecContext(ctx, `
UPDATE tunnels SET name=?,role=?,protocol=?,listen_addr=?,remote_addr=?,
 public_ports=?,local_services=?,secret_cipher=?,tls_cert=?,tls_key=?,
 tls_server_name=?,tls_ca_cert=?,skip_tls_verify=?,autostart=?,updated_at=?
WHERE id=?`,
		t.Name, t.Role, t.Protocol, t.ListenAddr, t.RemoteAddr, string(publicPorts),
		string(localServices), t.SecretCipher, t.TLSCert, t.TLSKey, t.TLSServerName,
		t.TLSCACert, boolInt(t.SkipTLSVerify), boolInt(t.Autostart),
		time.Now().UTC().Format(time.RFC3339), t.ID)
	return err
}

func (s *Store) DeleteTunnel(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM tunnels WHERE id=? AND status!='running'`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("running tunnels must be stopped before deletion")
	}
	return nil
}

func (s *Store) UpdateRuntime(ctx context.Context, id string, rt Runtime) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE tunnels SET status=?,pid=?,active_connections=?,bytes_in=?,bytes_out=?,
last_error=?,updated_at=? WHERE id=?`,
		rt.Status, rt.PID, rt.ActiveConns, rt.BytesIn, rt.BytesOut,
		rt.LastError, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

func (s *Store) MarkStopped(ctx context.Context, id, lastError string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE tunnels SET status='stopped',pid=0,active_connections=0,last_error=?,updated_at=?
WHERE id=?`, lastError, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

func (s *Store) ResetStaleRuntimes(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE tunnels SET status='stopped',pid=0,active_connections=0
WHERE status IN ('running','starting','stopping')`)
	return err
}

func (s *Store) Backup(ctx context.Context) ([]byte, error) {
	tunnels, err := s.ListTunnels(ctx)
	if err != nil {
		return nil, err
	}
	for i := range tunnels {
		tunnels[i].PID = 0
		tunnels[i].Status = "stopped"
		tunnels[i].ActiveConns = 0
	}
	return json.MarshalIndent(struct {
		Version int      `json:"version"`
		Date    string   `json:"date"`
		Tunnels []Tunnel `json:"tunnels"`
	}{1, time.Now().UTC().Format(time.RFC3339), tunnels}, "", "  ")
}

const tunnelSelect = `
SELECT id,name,role,protocol,listen_addr,remote_addr,public_ports,local_services,
secret_cipher,tls_cert,tls_key,tls_server_name,tls_ca_cert,skip_tls_verify,
autostart,status,pid,active_connections,bytes_in,bytes_out,last_error,created_at,updated_at
FROM tunnels`

type scanner interface {
	Scan(dest ...any) error
}

func scanTunnel(row scanner) (Tunnel, error) {
	var t Tunnel
	var publicPorts, localServices string
	var skipVerify, autostart int
	err := row.Scan(
		&t.ID, &t.Name, &t.Role, &t.Protocol, &t.ListenAddr, &t.RemoteAddr,
		&publicPorts, &localServices, &t.SecretCipher, &t.TLSCert, &t.TLSKey,
		&t.TLSServerName, &t.TLSCACert, &skipVerify, &autostart, &t.Status,
		&t.PID, &t.ActiveConns, &t.BytesIn, &t.BytesOut, &t.LastError,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return t, err
	}
	t.SkipTLSVerify = skipVerify == 1
	t.Autostart = autostart == 1
	if err := json.Unmarshal([]byte(publicPorts), &t.PublicPorts); err != nil {
		return t, fmt.Errorf("decode public ports: %w", err)
	}
	if err := json.Unmarshal([]byte(localServices), &t.LocalServices); err != nil {
		return t, fmt.Errorf("decode local services: %w", err)
	}
	return t, nil
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
