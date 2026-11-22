package config

import "os"

// Config holds everything the app reads from the environment at startup.
// Grouping it into one struct (instead of calling os.Getenv everywhere)
// makes it obvious what the app depends on and makes testing easier later.
type Config struct {
	DatabaseURL string
	RedisURL    string
	Port        string
}

// Load reads env vars into a Config, falling back to local-dev defaults
// when a var isn't set. In production you'd set real env vars (e.g. via
// Docker, Kubernetes, or a platform like Render/Fly.io) and these fallbacks
// would never be used.
func Load() Config {
	return Config{
		DatabaseURL: getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/urlshortener?sslmode=disable"),
		RedisURL:    getEnv("REDIS_URL", "redis://localhost:6379/0"),
		Port:        getEnv("PORT", "8080"),
	}
}

// getEnv is a small helper since os.Getenv returns "" (not an error) for a
// missing var — there's no built-in "getenv with default" in the stdlib.
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
