package model

import "time"

// Link is a domain struct: a plain data holder with no behavior.
// The `json:"..."` text after each field is a "struct tag" — metadata read
// by reflection at runtime. encoding/json uses it to decide what key name
// to use when marshaling this struct to/from JSON (otherwise it would use
// the Go field name verbatim, e.g. "LongURL" instead of "long_url").
type Link struct {
	ID        int64     `json:"id"`
	Code      string    `json:"code"`
	LongURL   string    `json:"long_url"`
	CreatedAt time.Time `json:"created_at"`
}

// Click records a single redirect event for a Link.
type Click struct {
	ID        int64     `json:"id"`
	LinkID    int64     `json:"link_id"` // foreign key, matches Link.ID
	Timestamp time.Time `json:"timestamp"`
	Referrer  string    `json:"referrer"`
	UserAgent string    `json:"user_agent"`
}
