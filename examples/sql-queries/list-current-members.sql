-- name: ListCurrentMembers :many
SELECT m.member_id, m.contact_email, m.label
FROM members AS m
JOIN organizations AS o ON o.organization_id = m.organization_id
WHERE m.organization_id = $1
  AND m.archived_at IS NULL
  AND o.disabled_at IS NULL
ORDER BY m.joined_at DESC
;
