# URL Shortener + Analytics API (Go)

## Context
The user is learning Go and has a minimal `tutorials/main.go` using `gorilla/mux` with in-memory data. They want a "proper" Go backend project to practice real-world structure. They chose: REST API + database, intermediate comfort with Go basics, and a **URL shortener with click analytics** as the project — scoped smaller than a full e-commerce app but still touching DB persistence, caching, unique code generation, and analytics aggregation.

## Goal
Scaffold a new, properly structured Go project (separate from `tutorials/`) implementing:
- `POST /links` — create a short link for a given long URL (generate a short code, e.g. base62 random or hash-based)
- `GET /{code}` — redirect to the original URL, and record a click event (timestamp, referrer, user-agent)
- `GET /links/{code}/stats` — return click analytics (total clicks, clicks over time, top referrers)
- `GET /links` — list links (paginated)

## Structure
Create a new top-level module directory, e.g. `urlshortener/`, with idiomatic layering:
```
urlshortener/
  cmd/server/main.go        // wiring: config, DB, router, start server
  internal/
    handler/                // HTTP handlers (thin, call service layer)
    service/                // business logic (code generation, click recording)
    repository/             // DB access (Postgres via pgx or database/sql)
    model/                   // domain structs (Link, Click)
  migrations/                // SQL migration files (e.g. via golang-migrate)
  go.mod
  docker-compose.yml        // Postgres (+ optional Redis) for local dev
  README.md                 // setup + run instructions
```

## Key decisions
- **DB**: Postgres, accessed via `pgx` (idiomatic, no heavy ORM) — good for learning raw SQL and query patterns. Migrations via `golang-migrate`.
- **Router**: keep `gorilla/mux` (already familiar) or switch to stdlib `net/http` with Go 1.22+ routing patterns — recommend stdlib to learn what mux abstracts, but keep mux as a fallback if the user prefers consistency with the existing tutorial.
- **Short code generation**: random base62 string (6-7 chars) with DB uniqueness check/retry — simple and teaches collision handling.
- **Click analytics**: separate `clicks` table (link_id, timestamp, referrer, user_agent), aggregated via SQL `GROUP BY` for stats endpoint — avoids needing Redis initially; can mention Redis as a stretch goal for caching hot redirects.
- **Testing**: table-driven unit tests for service layer (code generation, validation) + one integration test for the repository using a test Postgres instance (or `sqlmock`).

## Implementation steps
1. Scaffold directory structure and `go.mod` for `urlshortener` module.
2. Write `model` structs (`Link`, `Click`).
3. Write migration SQL for `links` and `clicks` tables.
4. Implement `repository` layer (CRUD for links, insert click, aggregate stats query).
5. Implement `service` layer (create link w/ code generation + collision retry, record click, compute stats).
6. Implement `handler` layer (JSON request/response, route wiring).
7. Wire up `cmd/server/main.go`: load config (env vars for DB DSN, port), connect DB, build router, start server.
8. Add `docker-compose.yml` for local Postgres.
9. Add README with setup/run/test instructions.
10. Write basic unit tests for the service layer.

## Verification
- `docker-compose up -d` starts Postgres; run migrations.
- `go run ./cmd/server` starts the API.
- `curl -X POST localhost:8080/links -d '{"url":"https://example.com"}'` returns a short code.
- `curl -L localhost:8080/{code}` redirects and records a click.
- `curl localhost:8080/links/{code}/stats` returns click counts.
- `go test ./...` passes.
