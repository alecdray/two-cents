package app

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Env string

const (
	EnvUnknown Env = "unknown"
	EnvLocal   Env = "local"
	EnvProd    Env = "prod"
)

func NewEnv(env string) Env {
	switch env {
	case "local":
		return EnvLocal
	case "prod":
		return EnvProd
	default:
		return EnvUnknown
	}
}

func init() {
	if err := godotenv.Load(); err != nil {
		slog.Warn("Unable to load .env file")
	}
}

type Config struct {
	Env        Env
	Port       string
	DbPath     string
	Host       string
	AppName    string
	AppVersion string

	// EncryptionKey is the hex-encoded symmetric key used to protect secrets at
	// rest (e.g. stored bank access tokens). Required — there is no sensible
	// default for a secret.
	EncryptionKey string

	Plaid PlaidConfig
}

// PlaidConfig holds the Plaid credentials and request defaults read from the
// environment. ClientID and Secret are required; the rest carry sensible
// defaults.
type PlaidConfig struct {
	ClientID     string
	Secret       string
	Env          string
	CountryCodes []string
	Products     []string
}

func LoadConfig() *Config {
	env := NewEnv(GetEnvWithDefault("ENV", "local"))
	port := GetEnvWithDefault("PORT", "4690")
	host := GetEnvWithConditionalPanic("HOST", fmt.Sprintf("http://127.0.0.1:%s", port), env != EnvLocal)

	return &Config{
		Env:           env,
		Port:          port,
		DbPath:        GetEnvWithDefault("DB_PATH", "./tmp/db.sql"),
		Host:          host,
		AppName:       GetEnvWithDefault("APP_NAME", "Two Cents"),
		AppVersion:    GetEnvWithDefault("APP_VERSION", "0.0.0"),
		EncryptionKey: GetEnvWithPanic("ENCRYPTION_KEY"),
		Plaid: PlaidConfig{
			ClientID:     GetEnvWithPanic("PLAID_CLIENT_ID"),
			Secret:       GetEnvWithPanic("PLAID_SECRET"),
			Env:          GetEnvWithDefault("PLAID_ENV", "production"),
			CountryCodes: splitAndTrim(GetEnvWithDefault("PLAID_COUNTRY_CODES", "US")),
			Products:     splitAndTrim(GetEnvWithDefault("PLAID_PRODUCTS", "transactions")),
		},
	}
}

// splitAndTrim turns a comma-separated env value into a slice, dropping blanks
// and surrounding whitespace.
func splitAndTrim(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func GetEnvWithPanic(key string) string {
	value := os.Getenv(key)
	if value == "" {
		panic(fmt.Sprintf("environment variable %s not set", key))
	}
	return value
}

func GetEnvWithDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func GetEnvWithConditionalPanic(key, defaultValue string, condition bool) string {
	if condition {
		return GetEnvWithPanic(key)
	}
	return GetEnvWithDefault(key, defaultValue)
}
