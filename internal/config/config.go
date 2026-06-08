package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jholhewres/anchored_oss/internal/ai/chat"
	"github.com/jholhewres/anchored_oss/internal/ai/embeddings"
	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Server     ServerConfig      `yaml:"server"`
	Database   DatabaseConfig    `yaml:"database"`
	CORS       CORSConfig        `yaml:"cors"`
	Quota      QuotaConfig       `yaml:"quota"`
	Curation   CurationConfig    `yaml:"curation"`
	RateLimit  RateLimitConfig   `yaml:"rate_limit"`
	Audit      AuditConfig       `yaml:"audit"`
	Embeddings embeddings.Config `yaml:"embeddings"`
	Chat       chat.Config       `yaml:"chat"`
}

// RateLimitConfig controls the per-client token-bucket limiter. The global
// limit applies to every request; the auth limit is a stricter bucket guarding
// login/register against brute force. Set a RequestsPerMinute to 0 to disable
// that bucket.
type RateLimitConfig struct {
	Enabled               bool `yaml:"enabled"`
	RequestsPerMinute     int  `yaml:"requests_per_minute"`
	Burst                 int  `yaml:"burst"`
	AuthRequestsPerMinute int  `yaml:"auth_requests_per_minute"`
	AuthBurst             int  `yaml:"auth_burst"`
}

// AuditConfig bounds audit_log growth. A purge sweep deletes entries older than
// RetentionDays at PurgeInterval. RetentionDays <= 0 disables purging (keep
// everything).
type AuditConfig struct {
	RetentionDays int           `yaml:"retention_days"`
	PurgeInterval time.Duration `yaml:"purge_interval"`
}

type CurationConfig struct {
	WorkerEnabled    bool          `yaml:"worker_enabled"`
	BatchSize        int           `yaml:"batch_size"`
	Interval         time.Duration `yaml:"interval"`
	NearDupWindow    time.Duration `yaml:"near_dup_window"`
	NearDupThreshold float64       `yaml:"near_dup_threshold"`

	// Curation v2 (staleness + contradiction candidates). Both are advisory:
	// the worker only marks metadata, it never deletes.
	StaleAfterDays         int  `yaml:"stale_after_days"`        // age past which an unpinned, non-superseded memory is marked stale; <=0 disables
	ContradictionDetection bool `yaml:"contradiction_detection"` // enable contradiction-candidate marking
}

type ServerConfig struct {
	Address      string        `yaml:"address"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
}

type DatabaseConfig struct {
	Driver          string        `yaml:"driver"`
	DSN             string        `yaml:"dsn"`
	MaxOpenConns    int           `yaml:"max_open_conns"`
	MaxIdleConns    int           `yaml:"max_idle_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
}

type CORSConfig struct {
	AllowedOrigins []string `yaml:"allowed_origins"`
}

type QuotaConfig struct {
	MaxStorageBytes int64 `yaml:"max_storage_bytes"` // 0 = unlimited
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
		Quota: QuotaConfig{
			MaxStorageBytes: 0,
		},
		Curation: CurationConfig{
			WorkerEnabled:          true,
			BatchSize:              100,
			Interval:               5 * time.Second,
			NearDupWindow:          720 * time.Hour,
			NearDupThreshold:       0.85,
			StaleAfterDays:         180,
			ContradictionDetection: true,
		},
		RateLimit: RateLimitConfig{
			Enabled:               true,
			RequestsPerMinute:     600,
			Burst:                 120,
			AuthRequestsPerMinute: 10,
			AuthBurst:             5,
		},
		Audit: AuditConfig{
			RetentionDays: 90,
			PurgeInterval: 6 * time.Hour,
		},
		Embeddings: embeddings.DefaultConfig(),
		Chat:       chat.DefaultConfig(),
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
	if v := os.Getenv("RATE_LIMIT_ENABLED"); v != "" {
		cfg.RateLimit.Enabled = v == "1" || strings.EqualFold(v, "true")
	}
	if v := os.Getenv("RATE_LIMIT_RPM"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.RateLimit.RequestsPerMinute = n
		}
	}
	if v := os.Getenv("AUDIT_RETENTION_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Audit.RetentionDays = n
		}
	}
	// Embeddings env overrides let env-only deployments (no config.yaml) select
	// the provider — e.g. EMBEDDINGS_PROVIDER=onnx with a staged model dir.
	if v := os.Getenv("EMBEDDINGS_PROVIDER"); v != "" {
		cfg.Embeddings.Provider = v
	}
	if v := os.Getenv("EMBEDDINGS_MODEL"); v != "" {
		cfg.Embeddings.Model = v
	}
	if v := os.Getenv("EMBEDDINGS_MODEL_DIR"); v != "" {
		cfg.Embeddings.ModelDir = v
	}
	if v := os.Getenv("EMBEDDINGS_DIMENSIONS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Embeddings.Dimensions = n
		}
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
