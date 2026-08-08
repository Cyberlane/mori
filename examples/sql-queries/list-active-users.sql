-- name: ListActiveUsers :many
SELECT u.id, u.email, u.display_name
FROM users AS u
JOIN tenants AS t ON t.id = u.tenant_id
WHERE u.tenant_id = $1
  AND u.deleted_at IS NULL
  AND t.suspended_at IS NULL
ORDER BY u.created_at DESC
;
