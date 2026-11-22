CREATE TABLE links (
    id         BIGSERIAL PRIMARY KEY,
    code       TEXT NOT NULL,
    long_url   TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_links_code ON links (code);

CREATE TABLE clicks (
    id         BIGSERIAL PRIMARY KEY,
    link_id    BIGINT NOT NULL REFERENCES links (id) ON DELETE CASCADE,
    timestamp  TIMESTAMPTZ NOT NULL DEFAULT now(),
    referrer   TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_clicks_link_id_timestamp ON clicks (link_id, timestamp);
