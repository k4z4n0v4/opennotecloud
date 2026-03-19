package main

import (
	"fmt"
	"os"
	"strings"
	"syscall"
)

type Config struct {
	Listen    string // HTTP listen address
	DataDir   string // Root directory for file storage
	DBPath    string // SQLite database path
	BaseURL   string // External URL for signed URLs
	JWTSecret string
}

func loadConfig() *Config {
	cfg := &Config{
		Listen:  envOr("OPENNOTE_LISTEN", ":8080"),
		DataDir: envOr("OPENNOTE_DATA_DIR", "/data/files"),
		DBPath:  envOr("OPENNOTE_DB_PATH", "/data/opennotecloud.db"),
		BaseURL: os.Getenv("OPENNOTE_BASE_URL"),
	}

	if cfg.BaseURL == "" {
		port := cfg.Listen
		if strings.HasPrefix(port, ":") {
			port = port[1:]
		}
		cfg.BaseURL = fmt.Sprintf("http://localhost:%s", port)
	}

	return cfg
}

func diskTotalBytes(path string) int64 {
	var stat syscall.Statfs_t
	if syscall.Statfs(path, &stat) != nil {
		return 0
	}
	return int64(stat.Blocks) * int64(stat.Bsize)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
