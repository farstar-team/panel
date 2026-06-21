package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

type Config struct {
	DataDir      string
	DBPath       string
	LogDir       string
	MasterKey    string
	ListenAddr   string
	TLSCert      string
	TLSKey       string
	CookieSecure bool
}

func Load() Config {
	dataDir := env("FARSTAR_DATA_DIR", defaultDataDir())
	return Config{
		DataDir:      dataDir,
		DBPath:       filepath.Join(dataDir, "farstar.db"),
		LogDir:       filepath.Join(dataDir, "logs"),
		MasterKey:    filepath.Join(dataDir, "master.key"),
		ListenAddr:   env("FARSTAR_LISTEN", "127.0.0.1:8080"),
		TLSCert:      os.Getenv("FARSTAR_TLS_CERT"),
		TLSKey:       os.Getenv("FARSTAR_TLS_KEY"),
		CookieSecure: os.Getenv("FARSTAR_COOKIE_SECURE") == "true",
	}
}

func (c Config) EnsureDirs() error {
	if err := os.MkdirAll(c.DataDir, 0700); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	if err := os.MkdirAll(c.LogDir, 0700); err != nil {
		return fmt.Errorf("create log directory: %w", err)
	}
	return nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func defaultDataDir() string {
	if runtime.GOOS != "windows" {
		return "/etc/farstar"
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "farstar")
	}
	return ".farstar"
}
