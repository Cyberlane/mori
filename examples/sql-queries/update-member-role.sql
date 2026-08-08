-- name: UpdateMemberRole :exec
UPDATE members
SET role = $1, updated_at = CURRENT_TIMESTAMP
WHERE member_id = $2
  AND organization_id = $3;
