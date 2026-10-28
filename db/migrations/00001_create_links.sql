-- +goose Up
CREATE TABLE links (
    id           BIGSERIAL PRIMARY KEY,
    original_url TEXT        NOT NULL,
    short_name   VARCHAR(32) NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT links_short_name_key UNIQUE (short_name)
);

-- +goose Down
DROP TABLE links;
