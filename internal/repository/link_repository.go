package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"urlshortener/internal/model"
)

// Sentinel errors: pre-declared error values the caller can compare against
// with errors.Is(err, ErrLinkNotFound), instead of string-matching error
// messages. This is the idiomatic Go way to let callers distinguish
// "not found" from "something else went wrong" without a custom error type.
var ErrCodeTaken = errors.New("code already exists")
var ErrLinkNotFound = errors.New("link not found")

// LinkRepository wraps a Postgres connection pool and knows how to read/write
// the `links` table. It has no business logic — that's the service layer's
// job (added in Phase 2). Keeping SQL isolated here means the rest of the
// app never imports pgx directly.
type LinkRepository struct {
	pool *pgxpool.Pool
}

// NewLinkRepository is a constructor function. Go has no "new" keyword for
// custom types with setup logic, so the convention is a `NewXxx` function
// that returns a pointer to the initialized struct.
func NewLinkRepository(pool *pgxpool.Pool) *LinkRepository {
	return &LinkRepository{pool: pool}
}

// Create inserts a new link row and returns it fully populated (including
// the DB-assigned ID and created_at timestamp, via RETURNING).
// Methods are declared as `func (r *LinkRepository) Create(...)` — the
// `(r *LinkRepository)` part is the "receiver": it's what makes this a
// method on LinkRepository rather than a free-standing function.
func (r *LinkRepository) Create(ctx context.Context, code, longURL string) (model.Link, error) {
	var link model.Link
	err := r.pool.QueryRow(ctx,
		`INSERT INTO links (code, long_url) VALUES ($1, $2) RETURNING id, code, long_url, created_at`,
		code, longURL,
	).Scan(&link.ID, &link.Code, &link.LongURL, &link.CreatedAt)
	if err != nil {
		// pgErr lets us inspect the Postgres-specific error code. "23505" is
		// Postgres's code for a unique_violation — i.e. our code collided
		// with an existing one. errors.As "unwraps" err looking for a
		// *pgconn.PgError, similar to a type-safe type assertion.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return model.Link{}, ErrCodeTaken
		}
		return model.Link{}, err
	}
	return link, nil
}

// GetByCode looks up a link by its short code.
func (r *LinkRepository) GetByCode(ctx context.Context, code string) (model.Link, error) {
	var link model.Link
	err := r.pool.QueryRow(ctx,
		`SELECT id, code, long_url, created_at FROM links WHERE code = $1`,
		code,
	).Scan(&link.ID, &link.Code, &link.LongURL, &link.CreatedAt)
	if err != nil {
		// pgx.ErrNoRows is returned when the query matched zero rows —
		// not a real failure, just "doesn't exist". We translate it into
		// our own sentinel so callers don't need to import pgx.
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Link{}, ErrLinkNotFound
		}
		return model.Link{}, err
	}
	return link, nil
}

// List returns a page of links, most recent first.
func (r *LinkRepository) List(ctx context.Context, limit, offset int) ([]model.Link, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, code, long_url, created_at FROM links ORDER BY id DESC LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	// defer schedules rows.Close() to run when this function returns,
	// regardless of which return statement triggers it. This guarantees the
	// underlying DB connection resources are released even if we return an
	// error partway through reading rows below.
	defer rows.Close()

	// var links []model.Link starts as a nil slice (not an empty slice) —
	// that's fine in Go; appending to a nil slice works, and if there are
	// zero rows we'll correctly return nil, which encodes to JSON `null`/`[]`
	// equivalently for most clients.
	var links []model.Link
	for rows.Next() {
		var link model.Link
		if err := rows.Scan(&link.ID, &link.Code, &link.LongURL, &link.CreatedAt); err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	// rows.Err() must be checked after the loop: rows.Next() returns false
	// both when iteration finishes normally AND when an error occurred, so
	// this is how you tell the two cases apart.
	return links, rows.Err()
}
