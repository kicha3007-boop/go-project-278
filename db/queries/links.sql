-- name: CreateLink :one
INSERT INTO links (original_url, short_name)
VALUES ($1, $2)
RETURNING *;

-- name: GetLink :one
SELECT * FROM links
WHERE id = $1;

-- name: GetLinkByShortName :one
SELECT * FROM links
WHERE short_name = $1;

-- name: UpdateLink :one
UPDATE links
SET original_url = $2, short_name = $3
WHERE id = $1
RETURNING *;

-- name: DeleteLink :execrows
DELETE FROM links
WHERE id = $1;

-- name: ListLinks :many
-- Без page_limit (NULL) Postgres отдаёт все строки начиная с page_offset.
SELECT * FROM links
ORDER BY id
LIMIT sqlc.narg('page_limit') OFFSET sqlc.arg('page_offset');

-- name: CountLinks :one
SELECT count(*) FROM links;
