package app

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// defaultAppTimezone is the IANA zone used when APP_TIMEZONE is unset or names a
// zone the system cannot load. It is the ADR-0004 "EST" default.
const defaultAppTimezone = "America/New_York"

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

	// JwtSecret signs the single-local-login session token (ADR-0007). Required
	// outside local dev; in local dev it falls back to a dummy so the app boots
	// without configuration.
	JwtSecret string

	// BankProvider selects which bank provider the composition root injects:
	// "plaid" (the default) reaches the live bank network; "fake" is the
	// deterministic in-process stand-in used to exercise the connection flows
	// without a real provider. See ADR-0006.
	BankProvider string

	// AppTimezone is the single configured application timezone (ADR-0004): the
	// zone "today", "days left", and "current month" are reckoned in, available to
	// background jobs — not a per-request browser zone. Loaded from APP_TIMEZONE
	// (an IANA name), defaulting to America/New_York; an unloadable name falls back
	// to the default with a logged warning rather than failing startup.
	AppTimezone *time.Location

	// FixedSafetyMargin is the minimum dollar cushion kept in checking after a
	// sweep (the floor that prevents sweeping the account dry). Loaded from
	// FIXED_SAFETY_MARGIN (dollars, default 500).
	FixedSafetyMargin float64

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
	plaidEnv := GetEnvWithDefault("PLAID_ENV", "production")

	return &Config{
		Env:           env,
		Port:          port,
		DbPath:        GetEnvWithDefault("DB_PATH", "./tmp/db.sql"),
		Host:          host,
		AppName:       GetEnvWithDefault("APP_NAME", "Two Cents"),
		AppVersion:    GetEnvWithDefault("APP_VERSION", "0.0.0"),
		EncryptionKey: GetEnvWithPanic("ENCRYPTION_KEY"),
		JwtSecret:     GetEnvWithConditionalPanic("JWT_SECRET", "local-dev-secret", env != EnvLocal),
		BankProvider:      GetEnvWithDefault("BANK_PROVIDER", "plaid"),
		AppTimezone:       loadAppTimezone(),
		FixedSafetyMargin: loadFixedSafetyMargin(),
		Plaid: PlaidConfig{
			ClientID:     GetEnvWithPanic("PLAID_CLIENT_ID"),
			Secret:       plaidSecret(plaidEnv),
			Env:          plaidEnv,
			CountryCodes: splitAndTrim(GetEnvWithDefault("PLAID_COUNTRY_CODES", "US")),
			Products:     splitAndTrim(GetEnvWithDefault("PLAID_PRODUCTS", "transactions")),
		},
	}
}

// loadAppTimezone reads APP_TIMEZONE (an IANA name, defaulting to
// defaultAppTimezone) and loads it into a *time.Location. A name the system
// cannot load (a typo, or missing zoneinfo) falls back to the default with a
// logged warning rather than panicking — a bad timezone must not stop startup.
func loadAppTimezone() *time.Location {
	name := GetEnvWithDefault("APP_TIMEZONE", defaultAppTimezone)
	loc, err := time.LoadLocation(name)
	if err != nil {
		slog.Warn("invalid APP_TIMEZONE, falling back to default", "value", name, "default", defaultAppTimezone, "err", err)
		loc, err = time.LoadLocation(defaultAppTimezone)
		if err != nil {
			// The default itself is unloadable (no zoneinfo at all); UTC keeps the
			// app running with a sane zone rather than a nil location.
			slog.Warn("default timezone unloadable, falling back to UTC", "default", defaultAppTimezone, "err", err)
			return time.UTC
		}
	}
	return loc
}

// loadFixedSafetyMargin reads FIXED_SAFETY_MARGIN (dollars, default "500") and
// parses it as a float. An unparseable value falls back to 500 with a logged
// warning so a misconfiguration never prevents startup.
func loadFixedSafetyMargin() float64 {
	const defaultMargin = 500.0
	raw := GetEnvWithDefault("FIXED_SAFETY_MARGIN", "500")
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		slog.Warn("invalid FIXED_SAFETY_MARGIN, falling back to default", "value", raw, "default", defaultMargin, "err", err)
		return defaultMargin
	}
	return v
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

// plaidSecret resolves the Plaid secret for the active Plaid environment so a
// sandbox and a production secret can coexist in the environment and PLAID_ENV
// selects between them. It prefers the environment-suffixed var
// (e.g. PLAID_SECRET_SANDBOX, PLAID_SECRET_PRODUCTION) and falls back to the
// unsuffixed PLAID_SECRET, which keeps single-secret deployments working.
func plaidSecret(plaidEnv string) string {
	if v := os.Getenv("PLAID_SECRET_" + strings.ToUpper(plaidEnv)); v != "" {
		return v
	}
	return GetEnvWithPanic("PLAID_SECRET")
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
