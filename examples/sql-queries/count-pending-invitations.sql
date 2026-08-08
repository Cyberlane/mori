-- name: CountPendingInvitations :one
SELECT COUNT(*)
FROM invitations
WHERE organization_id = $1
  AND accepted_at IS NULL;
