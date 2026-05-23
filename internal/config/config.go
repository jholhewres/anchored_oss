package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	CORS     CORSConfig     `yaml:"cors"`
}

type ServerConfig struct {
	Address      string        `yaml:"address"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
}

type DatabaseConfig struct {
	DSN             string        `yaml:"dsn"`
	MaxOpenConns    int           `yaml:"max_open_conns"`
	MaxIdleConns    int           `yaml:"max_idle_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
}

type CORSConfig struct {
	AllowedOrigins []string `yaml:"allowed_origins"`
}

func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Address:      ":8080",
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 300 * time.Second,
		},
		Database: DatabaseConfig{
			DSN:             "postgres://localhost:5433/anchored_oss?sslmode=disable",
			MaxOpenConns:    10,
			MaxIdleConns:    5,
			ConnMaxLifetime: 5 * time.Minute,
		},
		CORS: CORSConfig{
			AllowedOrigins: []string{"*"},
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

func Load(path string) (*Config, error) {
	_ = godotenv.Load()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file %s: %w", path, err)
	}

	expanded := expandEnv(string(data))

	cfg := DefaultConfig()
	if err := yaml.Unmarshal([]byte(expanded), cfg); err != nil {
		return nil, fmt.Errorf("parse config file %s: %w", path, err)
	}

	return cfg, nil
}

func LoadFromEnv() (*Config, error) {
	_ = godotenv.Load()

	cfg := DefaultConfig()

	if port := os.Getenv("PORT"); port != "" {
		cfg.Server.Address = ":" + port
	}
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		cfg.Database.DSN = dsn
	}
	if origins := os.Getenv("CORS_ALLOWED_ORIGINS"); origins != "" {
		cfg.CORS.AllowedOrigins = parseCSV(origins)
	}

	return cfg, nil
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

