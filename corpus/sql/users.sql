SELECT id, email
FROM users
WHERE status = $1
ORDER BY created_at DESC;
