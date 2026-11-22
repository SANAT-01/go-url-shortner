# URL Shortener + Click Analytics API

A small Go REST API: shorten URLs, redirect through them, and see click
analytics per link. Built as a learning project to practice idiomatic Go
project structure (handler → service → repository layering), Postgres via
`pgx`, and Redis caching.

## Prerequisites

- Go 1.27+
- Docker (for local Postgres + Redis)

## Setup

1. Start Postgres and Redis:

   ```sh
   docker-compose up -d
   ```

2. Apply the migration (creates the `links` and `clicks` tables):

   ```sh
   docker exec -i <postgres-container-name> psql -U postgres -d urlshortener < migrations/0001_init.up.sql
   ```

   Find the container name with `docker ps` if `docker-compose` didn't name
   it `go-url-shortner-postgres-1`.

3. Copy `.env.example` to `.env` (or just rely on its values — `config.Load`
   falls back to the same defaults if the env vars aren't set).

## Run

```sh
go run -buildvcs=false ./cmd/server
```

> The `-buildvcs=false` flag works around a `go build`/`go run` VCS-stamping
> error that can occur depending on local git configuration — harmless to
> pass even where it isn't needed.

The server listens on `:8080` by default (override with `PORT`).

## API

| Method | Path                    | Description                          |
|--------|--------------------------|---------------------------------------|
| POST   | `/links`                 | Create a short link                   |
| GET    | `/{code}`                | Redirect to the long URL, record click |
| GET    | `/links/{code}/stats`    | Click analytics for a link            |
| GET    | `/links`                 | List links (`?limit=&offset=`)        |

### Examples

```sh
# Create a short link
curl -X POST localhost:8080/links -d '{"url":"https://example.com"}'
# -> {"id":1,"code":"RqudJL","long_url":"https://example.com","created_at":"..."}

# Follow the short link (redirects, and records a click)
curl -i localhost:8080/RqudJL

# View click analytics
curl localhost:8080/links/RqudJL/stats
# -> {"total_clicks":1,"clicks_by_day":[...],"top_referrers":[...]}

# List all links
curl localhost:8080/links
```

## Test

```sh
go test -buildvcs=false ./...
```

Unit tests cover the service layer (`internal/service`) using in-memory
fakes for the repository/cache dependencies — no database required to run
them.
