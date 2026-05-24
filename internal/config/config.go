package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

// DefaultMode is set via:
//
//	-ldflags "-X github.com/jholhewres/anchored_oss/internal/config.DefaultMode=cloud"
//
// at build time. Defaults to "selfhosted" for the self-hosted binary.
var DefaultMode = "selfhosted"

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	CORS     CORSConfig     `yaml:"cors"`
	Mode     ModeConfig     `yaml:"mode"`
	Quota    QuotaConfig    `yaml:"quota"`
}

type ServerConfig struct {
	Address      string        `yaml:"address"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
}

type DatabaseConfig struct {
	Driver         string        `yaml:"driver"`
	DSN            string        `yaml:"dsn"`
	MaxOpenConns   int           `yaml:"max_open_conns"`
	MaxIdleConns   int           `yaml:"max_idle_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
}

type CORSConfig struct {
	AllowedOrigins []string `yaml:"allowed_origins"`
}

type ModeConfig struct {
	Type string `yaml:"type"` // "cloud" or "selfhosted" (default)
}

type QuotaConfig struct {
	MaxStorageBytes int64 `yaml:"max_storage_bytes"` // 0 = unlimited
}

// IsCloud returns true when the server runs in cloud (multi-tenant) mode.
func (c *Config) IsCloud() bool {
	return c.Mode.Type == "cloud"
}

// IsSelfHosted returns true when the server runs in self-hosted (single-tenant) mode.
// This is the default.
func (c *Config) IsSelfHosted() bool {
	return c.Mode.Type != "cloud"
}

// DefaultConfig returns conservative defaults intended for local dev only.
// CORS starts empty (no cross-origin access) and the DSN points at the
// docker-compose port.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Address:      ":8080",
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 300 * time.Second,
		},
		Database: DatabaseConfig{
			Driver:          "postgres",
			DSN:             "",
			MaxOpenConns:    10,
			MaxIdleConns:    5,
			ConnMaxLifetime: 5 * time.Minute,
		},
		CORS: CORSConfig{
			AllowedOrigins: nil,
		},
		Mode: ModeConfig{
			Type: DefaultMode,
		},
		Quota: QuotaConfig{
			MaxStorageBytes: 0,
		},
	}
}

// envVarRe matches ${VAR} and ${VAR:-default} patterns.
var envVarRe = regexp.MustCompile(`\$\{([a-zA-Z_][a-zA-Z0-9_]*)(?::-([^}]*))?\}`)

func expandEnv(s string) string {
	return envVarRe.ReplaceAllStringFunc(s, func(match string) string {
		parts := envVarRe.FindStringSubmatch(match)
		name := parts[1]
		def := parts[2]
		if val, ok := os.LookupEnv(name); ok {
			return val
		}
		return def
	})
}

// Load reads a YAML config file with ${ENV} substitution, then merges
// env-only overrides on top. Returns an error if neither a usable config
// file nor a DATABASE_URL is available.
func Load(path string) (*Config, error) {
	_ = godotenv.Load()

	cfg := DefaultConfig()
	loadedFromFile := false

	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		expanded := expandEnv(string(data))
		if err := yaml.Unmarshal([]byte(expanded), cfg); err != nil {
			return nil, fmt.Errorf("parse config file %s: %w", path, err)
		}
		loadedFromFile = true
	case errors.Is(err, os.ErrNotExist):
		// fall through to env overrides
	default:
		return nil, fmt.Errorf("read config file %s: %w", path, err)
	}

	applyEnvOverrides(cfg)

	if cfg.Database.DSN == "" {
		return nil, fmt.Errorf("database dsn is required: set database.dsn in %s or DATABASE_URL", path)
	}

	if !loadedFromFile {
		slog.Warn("config file not found; using env + defaults", "path", path)
	}
	return cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if port := os.Getenv("PORT"); port != "" {
		cfg.Server.Address = ":" + port
	}
	if driver := os.Getenv("DATABASE_DRIVER"); driver != "" {
		cfg.Database.Driver = driver
	}
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		cfg.Database.DSN = dsn
	}
	if origins := os.Getenv("CORS_ALLOWED_ORIGINS"); origins != "" {
		cfg.CORS.AllowedOrigins = parseCSV(origins)
	}
	if mode := os.Getenv("MODE"); mode != "" {
		cfg.Mode.Type = mode
	}
}

func parseCSV(s string) []string {
	var result []string
	for _, v := range strings.Split(s, ",") {
		v = strings.TrimSpace(v)
		if v != "" {
			result = append(result, v)
		}
	}
	return result
}
