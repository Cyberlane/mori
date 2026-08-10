SELECT account_id, contact_email
FROM accounts
WHERE state = $1
ORDER BY opened_at DESC;
