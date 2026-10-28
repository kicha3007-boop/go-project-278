-- name: CreateLinkVisit :one
INSERT INTO link_visits (link_id, ip, user_agent, referer, status)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListLinkVisits :many
SELECT * FROM link_visits
ORDER BY id
LIMIT sqlc.narg('page_limit') OFFSET sqlc.arg('page_offset');

-- name: CountLinkVisits :one
SELECT count(*) FROM link_visits;
