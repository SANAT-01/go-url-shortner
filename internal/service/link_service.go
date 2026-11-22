package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/url"

	"urlshortener/internal/model"
	"urlshortener/internal/repository"
)

// ErrInvalidURL is returned when the caller tries to shorten something that
// isn't a usable absolute URL.
var ErrInvalidURL = errors.New("invalid url")

// codeAlphabet is the character set used for generated short codes. 62
// characters (lower+upper+digits) is the "base62" alphabet — no symbols,
// so codes are always URL-safe without escaping.
const codeAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// codeLength is how many characters each generated short code has.
const codeLength = 6

// maxCreateAttempts bounds how many times we'll retry generating a code
// if we keep colliding with an existing one. With a 62^6 code space,
// collisions are rare, so this is just a safety valve, not a normal path.
const maxCreateAttempts = 5

// linkStore, clickStore, and urlCache are the small interfaces LinkService
// actually depends on — just the methods it calls, not the full
// repository.LinkRepository/ClickRepository/Cache types. *repository.LinkRepository
// etc. already satisfy these implicitly (Go interfaces are structural: no
// "implements" declaration needed), so main.go's wiring doesn't change.
// Defining them here, next to the consumer, lets tests substitute small
// in-memory fakes instead of a real Postgres/Redis connection.
type linkStore interface {
	Create(ctx context.Context, code, longURL string) (model.Link, error)
	GetByCode(ctx context.Context, code string) (model.Link, error)
	List(ctx context.Context, limit, offset int) ([]model.Link, error)
}

type clickStore interface {
	Insert(ctx context.Context, linkID int64, referrer, userAgent string) error
	AggregateStats(ctx context.Context, linkID int64) (repository.Stats, error)
}

type urlCache interface {
	GetURL(ctx context.Context, code string) (string, error)
	SetURL(ctx context.Context, code, url string) error
}

// LinkService contains the business logic that sits between HTTP handlers
// and the repository layer: generating short codes, deciding when to use
// the cache, and assembling analytics.
type LinkService struct {
	links  linkStore
	clicks clickStore
	cache  urlCache
}

func NewLinkService(links *repository.LinkRepository, clicks *repository.ClickRepository, cache *repository.Cache) *LinkService {
	return &LinkService{links: links, clicks: clicks, cache: cache}
}

// CreateLink validates longURL and persists it under a freshly generated
// short code, retrying on the rare collision.
func (s *LinkService) CreateLink(ctx context.Context, longURL string) (model.Link, error) {
	if !isValidURL(longURL) {
		return model.Link{}, ErrInvalidURL
	}

	for attempt := 0; attempt < maxCreateAttempts; attempt++ {
		code, err := generateCode(codeLength)
		if err != nil {
			return model.Link{}, err
		}

		link, err := s.links.Create(ctx, code, longURL)
		if err == nil {
			return link, nil
		}
		// A collision just means "try again with a new random code" — any
		// other error is unexpected and should propagate immediately.
		if !errors.Is(err, repository.ErrCodeTaken) {
			return model.Link{}, err
		}
	}

	return model.Link{}, fmt.Errorf("could not generate a unique code after %d attempts", maxCreateAttempts)
}

// Resolve looks up the link behind a short code, using the cache to skip
// the database for already-seen codes. It returns the full Link (not just
// the URL) because callers need the link's ID to record a click.
func (s *LinkService) Resolve(ctx context.Context, code string) (model.Link, error) {
	if cachedURL, err := s.cache.GetURL(ctx, code); err == nil && cachedURL != "" {
		// The cache only stores code->URL, not the ID, so a cache hit still
		// needs one DB round trip to get the Link.ID for click recording.
		// This keeps the cache's job simple (just the hot redirect value)
		// at the cost of not fully skipping the DB on hits.
		return s.links.GetByCode(ctx, code)
	}

	link, err := s.links.GetByCode(ctx, code)
	if err != nil {
		return model.Link{}, err
	}

	// Best-effort cache population: a cache write failure shouldn't fail
	// the redirect, so its error is deliberately ignored here.
	_ = s.cache.SetURL(ctx, link.Code, link.LongURL)

	return link, nil
}

// RecordClick stores one click event for a link.
func (s *LinkService) RecordClick(ctx context.Context, linkID int64, referrer, userAgent string) error {
	return s.clicks.Insert(ctx, linkID, referrer, userAgent)
}

// GetStats resolves a code to its link and returns its click analytics.
func (s *LinkService) GetStats(ctx context.Context, code string) (repository.Stats, error) {
	link, err := s.links.GetByCode(ctx, code)
	if err != nil {
		return repository.Stats{}, err
	}
	return s.clicks.AggregateStats(ctx, link.ID)
}

// ListLinks returns a page of links.
func (s *LinkService) ListLinks(ctx context.Context, limit, offset int) ([]model.Link, error) {
	return s.links.List(ctx, limit, offset)
}

// isValidURL requires an absolute URL with a scheme and host, e.g.
// "https://example.com" — rejects empty strings and relative paths.
func isValidURL(raw string) bool {
	if raw == "" {
		return false
	}
	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return false
	}
	return u.Scheme != "" && u.Host != ""
}

// generateCode returns a random base62 string of length n. It uses
// crypto/rand (not math/rand) because short codes are effectively a
// public identifier namespace — using a cryptographically secure source
// avoids codes being predictable.
func generateCode(n int) (string, error) {
	// buf starts as n random bytes (0-255 each); the loop below then maps
	// each byte onto one of the 62 alphabet characters.
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	// b % len(codeAlphabet) maps a random byte (0-255) onto a valid index
	// into codeAlphabet (0-61). This introduces a tiny, practically
	// irrelevant bias (256 isn't evenly divisible by 62), which is an
	// acceptable tradeoff for how simple it keeps this code — a
	// cryptographic use case would use rejection sampling instead.
	for i, b := range buf {
		buf[i] = codeAlphabet[int(b)%len(codeAlphabet)]
	}
	return string(buf), nil
}
