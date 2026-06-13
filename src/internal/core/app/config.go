package app

import (
	"fmt"
	"log/slog"
	"os"

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
}

func LoadConfig() *Config {
	env := NewEnv(GetEnvWithDefault("ENV", "local"))
	port := GetEnvWithDefault("PORT", "4690")
	host := GetEnvWithConditionalPanic("HOST", fmt.Sprintf("http://127.0.0.1:%s", port), env != EnvLocal)

	return &Config{
		Env:        env,
		Port:       port,
		DbPath:     GetEnvWithDefault("DB_PATH", "./tmp/db.sql"),
		Host:       host,
		AppName:    GetEnvWithDefault("APP_NAME", "Two Cents"),
		AppVersion: GetEnvWithDefault("APP_VERSION", "0.0.0"),
	}
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
