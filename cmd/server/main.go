package main

import (
	"context"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"urlshortener/internal/config"
	"urlshortener/internal/handler"
	"urlshortener/internal/repository"
	"urlshortener/internal/service"
)

// main is the entry point and the only place that wires concrete
// dependencies (a real Postgres pool, a real Redis client) into the
// layers below. Every other package only depends on interfaces/structs
// passed in from here, which is what makes them independently testable.
func main() {
	// context.Background() is the "root" context: one with no deadline and
	// no cancellation, used here only for the one-time startup calls below
	// (as opposed to a per-request context, which each HTTP handler gets
	// automatically from r.Context()).
	ctx := context.Background()
	cfg := config.Load()

	// pgxpool.New doesn't just open one connection — it manages a pool of
	// them, handing one out per query and returning it when done. That's
	// why every repository method takes a context but none of them "checks
	// out" a connection explicitly; the pool does that internally.
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect to postgres: %v", err)
	}
	// defer schedules pool.Close() for when main() returns normally. Note
	// that log.Fatalf calls os.Exit internally, which skips deferred calls —
	// so this only actually runs if main() ever returns via a graceful
	// shutdown path rather than a fatal error.
	defer pool.Close()

	// redis.ParseURL turns a "redis://host:port/db" URL into the fields
	// redis.NewClient's Options struct wants (Addr, DB, etc.), so cfg.RedisURL
	// (one string from the environment) doesn't have to be parsed by hand.
	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Fatalf("invalid redis url: %v", err)
	}
	redisClient := redis.NewClient(redisOpts)
	defer redisClient.Close()

	// Dependency wiring, bottom-up: repositories (DB/cache access) first,
	// then the service (business logic) that depends on them, then the
	// handler (HTTP layer) that depends on the service. Each layer only
	// knows about the one directly below it.
	linkRepo := repository.NewLinkRepository(pool)
	clickRepo := repository.NewClickRepository(pool)
	cache := repository.NewCache(redisClient)

	linkSvc := service.NewLinkService(linkRepo, clickRepo, cache)
	linkHandler := handler.NewLinkHandler(linkSvc)

	// Go 1.22+'s ServeMux supports "METHOD /path" patterns and named
	// wildcards like "{code}", retrievable via r.PathValue("code") — no
	// third-party router needed for routing this simple. Routes are matched
	// most-specific-first regardless of registration order, so
	// "/links/{code}/stats" is tried before the catch-all "/{code}".
	mux := http.NewServeMux()
	mux.HandleFunc("POST /links", linkHandler.CreateLink)
	mux.HandleFunc("GET /links", linkHandler.List)
	mux.HandleFunc("GET /links/{code}/stats", linkHandler.Stats)
	mux.HandleFunc("GET /{code}", linkHandler.Redirect)

	addr := ":" + cfg.Port
	log.Printf("listening on %s", addr)
	// ListenAndServe blocks forever (serving requests) until it hits an
	// error, e.g. the port is already in use — at which point there's
	// nothing more main() can do but log it and exit.
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
