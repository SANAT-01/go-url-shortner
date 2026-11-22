package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"urlshortener/internal/model"
	"urlshortener/internal/repository"
)

// --- fakes ---
//
// These are minimal in-memory stand-ins for linkStore/clickStore/urlCache,
// just enough behavior to drive the scenarios below. Because LinkService
// depends on interfaces (see link_service.go), nothing here touches a real
// database or Redis instance.

// fakeLinkStore lets a test script which Create calls should fail with
// repository.ErrCodeTaken before succeeding, to exercise CreateLink's
// collision-retry loop.
type fakeLinkStore struct {
	failFirstN int // number of Create calls that should return ErrCodeTaken
	calls      int
}

func (f *fakeLinkStore) Create(ctx context.Context, code, longURL string) (model.Link, error) {
	f.calls++
	if f.calls <= f.failFirstN {
		return model.Link{}, repository.ErrCodeTaken
	}
	return model.Link{ID: 1, Code: code, LongURL: longURL}, nil
}

func (f *fakeLinkStore) GetByCode(ctx context.Context, code string) (model.Link, error) {
	return model.Link{}, repository.ErrLinkNotFound
}

func (f *fakeLinkStore) List(ctx context.Context, limit, offset int) ([]model.Link, error) {
	return nil, nil
}

// fakeClickStore and fakeURLCache aren't exercised by the tests below, but
// LinkService's struct fields require something satisfying every method of
// clickStore/urlCache, so these stubs fill that role.
type fakeClickStore struct{}

func (fakeClickStore) Insert(ctx context.Context, linkID int64, referrer, userAgent string) error {
	return nil
}

func (fakeClickStore) AggregateStats(ctx context.Context, linkID int64) (repository.Stats, error) {
	return repository.Stats{}, nil
}

type fakeURLCache struct{}

func (fakeURLCache) GetURL(ctx context.Context, code string) (string, error) { return "", nil }
func (fakeURLCache) SetURL(ctx context.Context, code, url string) error      { return nil }

// --- tests ---

func TestIsValidURL(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want bool
	}{
		{"valid https", "https://example.com", true},
		{"valid http with path", "http://example.com/foo?bar=1", true},
		{"empty string", "", false},
		{"relative path", "/foo/bar", false},
		{"missing scheme", "example.com", false},
		{"scheme with no host", "file:///etc/passwd", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isValidURL(tc.url)
			if got != tc.want {
				t.Errorf("isValidURL(%q) = %v, want %v", tc.url, got, tc.want)
			}
		})
	}
}

func TestGenerateCode(t *testing.T) {
	code, err := generateCode(codeLength)
	if err != nil {
		t.Fatalf("generateCode returned error: %v", err)
	}
	if len(code) != codeLength {
		t.Fatalf("generateCode returned length %d, want %d", len(code), codeLength)
	}
	for _, c := range code {
		if !strings.ContainsRune(codeAlphabet, c) {
			t.Fatalf("generateCode produced character %q not in codeAlphabet", c)
		}
	}
}

func TestCreateLink_RetriesOnCollision(t *testing.T) {
	links := &fakeLinkStore{failFirstN: 2} // first two attempts collide, third succeeds
	svc := &LinkService{links: links, clicks: fakeClickStore{}, cache: fakeURLCache{}}

	link, err := svc.CreateLink(context.Background(), "https://example.com")
	if err != nil {
		t.Fatalf("CreateLink returned error: %v", err)
	}
	if links.calls != 3 {
		t.Errorf("expected 3 Create calls (2 collisions + 1 success), got %d", links.calls)
	}
	if link.LongURL != "https://example.com" {
		t.Errorf("link.LongURL = %q, want %q", link.LongURL, "https://example.com")
	}
}

func TestCreateLink_GivesUpAfterMaxAttempts(t *testing.T) {
	links := &fakeLinkStore{failFirstN: maxCreateAttempts} // every attempt collides
	svc := &LinkService{links: links, clicks: fakeClickStore{}, cache: fakeURLCache{}}

	_, err := svc.CreateLink(context.Background(), "https://example.com")
	if err == nil {
		t.Fatal("expected an error after exhausting all attempts, got nil")
	}
	if links.calls != maxCreateAttempts {
		t.Errorf("expected %d Create calls, got %d", maxCreateAttempts, links.calls)
	}
}

func TestCreateLink_RejectsInvalidURL(t *testing.T) {
	svc := &LinkService{links: &fakeLinkStore{}, clicks: fakeClickStore{}, cache: fakeURLCache{}}

	_, err := svc.CreateLink(context.Background(), "not-a-url")
	if !errors.Is(err, ErrInvalidURL) {
		t.Errorf("expected ErrInvalidURL, got %v", err)
	}
}
