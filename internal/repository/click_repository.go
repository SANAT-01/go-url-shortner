package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ClickRepository is the analytics-side counterpart to LinkRepository: it
// writes click events and runs the aggregate queries behind the stats
// endpoint.
type ClickRepository struct {
	pool *pgxpool.Pool
}

func NewClickRepository(pool *pgxpool.Pool) *ClickRepository {
	return &ClickRepository{pool: pool}
}

// Insert records one click. pool.Exec is used instead of QueryRow/Query
// because we don't need any rows back — just the error, if any.
func (r *ClickRepository) Insert(ctx context.Context, linkID int64, referrer, userAgent string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO clicks (link_id, referrer, user_agent) VALUES ($1, $2, $3)`,
		linkID, referrer, userAgent,
	)
	return err
}

// Stats is a response shape specific to this query, not a core domain
// model — that's why it lives here next to AggregateStats rather than in
// internal/model.
type Stats struct {
	TotalClicks  int64           `json:"total_clicks"`
	ClicksByDay  []DayCount      `json:"clicks_by_day"`
	TopReferrers []ReferrerCount `json:"top_referrers"`
}

type DayCount struct {
	Day   string `json:"day"`
	Count int64  `json:"count"`
}

type ReferrerCount struct {
	Referrer string `json:"referrer"`
	Count    int64  `json:"count"`
}

// AggregateStats runs three separate queries and assembles their results
// into one Stats value. Doing this in three queries (rather than one
// complex JOIN) keeps each SQL statement simple and easy to reason about —
// a fine tradeoff at this data scale.
func (r *ClickRepository) AggregateStats(ctx context.Context, linkID int64) (Stats, error) {
	var stats Stats

	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM clicks WHERE link_id = $1`, linkID,
	).Scan(&stats.TotalClicks); err != nil {
		return Stats{}, err
	}

	// date_trunc('day', timestamp) rounds each click's timestamp down to
	// midnight, so GROUP BY day effectively buckets clicks per calendar day.
	dayRows, err := r.pool.Query(ctx,
		`SELECT to_char(date_trunc('day', timestamp), 'YYYY-MM-DD') AS day, COUNT(*)
		 FROM clicks WHERE link_id = $1
		 GROUP BY day ORDER BY day`,
		linkID,
	)
	if err != nil {
		return Stats{}, err
	}
	defer dayRows.Close()
	for dayRows.Next() {
		var dc DayCount
		if err := dayRows.Scan(&dc.Day, &dc.Count); err != nil {
			return Stats{}, err
		}
		stats.ClicksByDay = append(stats.ClicksByDay, dc)
	}
	if err := dayRows.Err(); err != nil {
		return Stats{}, err
	}

	// Top 5 referrers by click count, descending.
	refRows, err := r.pool.Query(ctx,
		`SELECT referrer, COUNT(*) FROM clicks WHERE link_id = $1
		 GROUP BY referrer ORDER BY COUNT(*) DESC LIMIT 5`,
		linkID,
	)
	if err != nil {
		return Stats{}, err
	}
	defer refRows.Close()
	for refRows.Next() {
		var rc ReferrerCount
		if err := refRows.Scan(&rc.Referrer, &rc.Count); err != nil {
			return Stats{}, err
		}
		stats.TopReferrers = append(stats.TopReferrers, rc)
	}
	return stats, refRows.Err()
}
