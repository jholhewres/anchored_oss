package setup

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/jholhewres/anchored_oss/internal/auth"
	"github.com/jholhewres/anchored_oss/internal/store"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

type SetupConfig struct {
	Driver         string
	DSN            string
	OrgName        string
	AdminEmail     string
	AdminPassword  string
	AdminName      string
	Port           int
	ConfigPath     string
	DockerPostgres bool
}

type yamlConfig struct {
	Server   yamlServer   `yaml:"server"`
	Database yamlDatabase `yaml:"database"`
}

type yamlServer struct {
	Address string `yaml:"address"`
}

type yamlDatabase struct {
	Driver string `yaml:"driver"`
	DSN    string `yaml:"dsn"`
}

func RunInteractive() error {
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Println("=== Anchored OSS Setup Wizard ===")
	fmt.Println()

	driver := promptChoice(scanner, "Choose your database", []string{"sqlite", "postgres"}, "sqlite")

	var dsn string
	var dockerPostgres bool

	if driver == "sqlite" {
		dsn = promptLine(scanner, "Where should the database be stored?", "./anchored.db")
	} else {
		useDocker := promptChoice(scanner, "Use Docker for PostgreSQL?", []string{"yes", "no"}, "yes")
		if useDocker == "yes" {
			dockerPostgres = true
			generatedDSN, err := GenerateDockerCompose()
			if err != nil {
				return fmt.Errorf("generate docker-compose: %w", err)
			}
			dsn = generatedDSN
			fmt.Println()
			fmt.Println("Generated docker-compose.yml for PostgreSQL.")
			fmt.Println("Starting PostgreSQL...")
			fmt.Println()
			if err := startDockerCompose(); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not start docker compose: %v\n", err)
				fmt.Fprintf(os.Stderr, "Please run 'docker compose up -d' manually, then press Enter to continue.\n")
				scanner.Scan()
			} else {
				fmt.Println("Waiting for PostgreSQL to be ready...")
				if err := WaitForPostgres(dsn, 30*time.Second); err != nil {
					return fmt.Errorf("postgres health check: %w", err)
				}
				fmt.Println("PostgreSQL is ready.")
			}
		} else {
			dsn = promptPostgresDSN(scanner)
		}
	}

	orgName := promptLine(scanner, "Organization name:", "default")
	adminEmail := promptLine(scanner, "Admin email:", "admin@anchored.local")
	adminPassword := promptPassword(scanner, "Admin password", "changeme")
	adminName := promptLine(scanner, "Admin display name:", "Admin")
	portStr := promptLine(scanner, "Server port:", "8080")

	var port int
	fmt.Sscanf(portStr, "%d", &port)
	if port <= 0 {
		port = 8080
	}

	cfg := SetupConfig{
		Driver:         driver,
		DSN:            dsn,
		OrgName:        orgName,
		AdminEmail:     adminEmail,
		AdminPassword:  adminPassword,
		AdminName:      adminName,
		Port:           port,
		ConfigPath:     "config.yaml",
		DockerPostgres: dockerPostgres,
	}

	return RunNonInteractive(cfg)
}

func RunNonInteractive(cfg SetupConfig) error {
	if cfg.Driver == "" {
		cfg.Driver = "sqlite"
	}
	if cfg.ConfigPath == "" {
		cfg.ConfigPath = "config.yaml"
	}
	if cfg.Port <= 0 {
		cfg.Port = 8080
	}

	if err := writeConfigFile(cfg); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	slog.Info("wrote config file", "path", cfg.ConfigPath)

	st, err := openStore(cfg)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer st.Close()

	apiKey, err := bootstrapData(st, cfg)
	if err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}

	fmt.Println()
	fmt.Println("=== Setup Complete ===")
	fmt.Printf("Config file: %s\n", cfg.ConfigPath)
	fmt.Printf("Database:    %s (%s)\n", cfg.Driver, redactDSN(cfg.DSN))
	fmt.Printf("Org:         %s\n", cfg.OrgName)
	fmt.Printf("Admin:       %s (%s)\n", cfg.AdminEmail, cfg.AdminName)
	fmt.Printf("Server:      http://localhost:%d\n", cfg.Port)
	fmt.Println()
	fmt.Println("API Key (copy now — it cannot be retrieved later):")
	fmt.Println(apiKey)
	fmt.Println()

	return nil
}

func writeConfigFile(cfg SetupConfig) error {
	if _, err := os.Stat(cfg.ConfigPath); err == nil {
		return fmt.Errorf("config file %s already exists; remove it first or use a different path", cfg.ConfigPath)
	}

	out := yamlConfig{
		Server: yamlServer{Address: fmt.Sprintf(":%d", cfg.Port)},
		Database: yamlDatabase{
			Driver: cfg.Driver,
			DSN:    cfg.DSN,
		},
	}

	data, err := yaml.Marshal(out)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	return os.WriteFile(cfg.ConfigPath, data, 0o600)
}

func openStore(cfg SetupConfig) (store.Store, error) {
	if cfg.Driver == "sqlite" {
		return store.NewSQLiteStore(cfg.DSN)
	}
	return store.NewPostgresStore(store.PoolConfig{
		DSN:             cfg.DSN,
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
	})
}

func bootstrapData(s store.Store, cfg SetupConfig) (string, error) {
	ctx := context.Background()

	slug := slugify(cfg.OrgName)
	org, err := s.CreateOrganization(ctx, cfg.OrgName, slug)
	if err != nil {
		return "", fmt.Errorf("create organization: %w", err)
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(cfg.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}

	account, err := s.CreateAccount(ctx, cfg.AdminEmail, cfg.AdminName, string(passwordHash))
	if err != nil {
		return "", fmt.Errorf("create account: %w", err)
	}

	if err := s.AddOrgMember(ctx, org.ID, account.ID, "admin"); err != nil {
		return "", fmt.Errorf("add org member: %w", err)
	}

	if err := s.EnsureDefaultTeamMembership(ctx, org.ID, account.ID); err != nil {
		return "", fmt.Errorf("ensure default team membership: %w", err)
	}

	full, prefix, hash, err := auth.GenerateAPIKey()
	if err != nil {
		return "", fmt.Errorf("generate api key: %w", err)
	}

	if _, err := s.CreateAPIKey(ctx, org.ID, account.ID, "admin", prefix, hash, "admin", nil); err != nil {
		return "", fmt.Errorf("create api key: %w", err)
	}

	return full, nil
}

// promptPostgresDSN collects PostgreSQL connection fields one at a time and
// assembles a valid DSN, so the user never has to hand-craft a connection
// string. The password is URL-encoded via url.UserPassword, so special
// characters are handled safely.
func promptPostgresDSN(scanner *bufio.Scanner) string {
	host := promptLine(scanner, "PostgreSQL host", "localhost")
	port := promptLine(scanner, "PostgreSQL port", "5432")
	dbName := promptLine(scanner, "Database name", "anchored_oss")
	user := promptLine(scanner, "Database user", "anchored")
	password := promptPassword(scanner, "Database password (leave blank if none)", "")
	sslMode := promptChoice(scanner, "SSL mode", []string{"disable", "require", "verify-full"}, "disable")

	u := url.URL{
		Scheme: "postgres",
		Host:   net.JoinHostPort(host, port),
		Path:   "/" + dbName,
	}
	if password != "" {
		u.User = url.UserPassword(user, password)
	} else {
		u.User = url.User(user)
	}
	q := url.Values{}
	q.Set("sslmode", sslMode)
	u.RawQuery = q.Encode()
	return u.String()
}

// promptPassword reads a secret without echoing it to the terminal. When stdin
// is a real terminal (the normal interactive case, including `curl | sh` via
// /dev/tty) it uses term.ReadPassword; otherwise (piped/non-interactive input)
// it falls back to a normal scanner line so automation still works. A blank
// entry yields defaultVal.
func promptPassword(scanner *bufio.Scanner, prompt, defaultVal string) string {
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		fmt.Printf("%s: ", prompt)
		b, err := term.ReadPassword(fd)
		fmt.Println() // ReadPassword swallows the Enter; emit the newline ourselves
		if err != nil {
			return defaultVal
		}
		val := strings.TrimSpace(string(b))
		if val == "" {
			return defaultVal
		}
		return val
	}
	// Non-terminal: read a normal line (keeps piped setups working).
	return promptLine(scanner, prompt, defaultVal)
}

func promptLine(scanner *bufio.Scanner, prompt, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("%s [%s]: ", prompt, defaultVal)
	} else {
		fmt.Printf("%s: ", prompt)
	}
	if !scanner.Scan() {
		return defaultVal
	}
	val := strings.TrimSpace(scanner.Text())
	if val == "" {
		return defaultVal
	}
	return val
}

func promptChoice(scanner *bufio.Scanner, prompt string, options []string, defaultVal string) string {
	fmt.Printf("%s (%s", prompt, options[0])
	for _, o := range options[1:] {
		fmt.Printf("/%s", o)
	}
	fmt.Printf(") [%s]: ", defaultVal)

	if !scanner.Scan() {
		return defaultVal
	}
	val := strings.TrimSpace(strings.ToLower(scanner.Text()))
	if val == "" {
		return defaultVal
	}
	for _, o := range options {
		if strings.EqualFold(val, o) {
			return o
		}
	}
	return defaultVal
}

var wizardStripChars = strings.NewReplacer(
	"!", "", "@", "", "#", "", "$", "", "%", "", "^", "", "&", "", "*", "",
	"(", "", ")", "", "+", "", "=", "", "{", "", "}", "", "[", "", "]", "",
	"|", "", "\\", "", ":", "", ";", "", "'", "", "<", "", ">", "",
	",", "", "?", "", "/", "", "~", "", "`", "", "\"", "",
)

func slugify(name string) string {
	s := strings.ToLower(name)
	s = strings.ReplaceAll(s, " ", "-")
	s = wizardStripChars.Replace(s)
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	s = strings.Trim(s, "-")
	return s
}

func redactDSN(dsn string) string {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		parts := strings.SplitN(dsn, "@", 2)
		if len(parts) == 2 {
			return "postgres://***@" + parts[1]
		}
	}
	return dsn
}

func startDockerCompose() error {
	cmd := exec.Command("docker", "compose", "up", "-d")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
