-- name: UpsertSubscription :exec
INSERT INTO calendar_subscriptions (calendar_id, user_agent, ip_hash)
VALUES ($1, $2, $3)
ON CONFLICT (calendar_id, ip_hash) WHERE ip_hash IS NOT NULL
DO UPDATE SET
    last_request_at = NOW(),
    request_count = calendar_subscriptions.request_count + 1,
    user_agent = EXCLUDED.user_agent;
