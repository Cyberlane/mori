-- name: ArchiveExpired :exec
UPDATE sessions
SET archived = TRUE
WHERE expires_at < NOW()
RETURNING id;
