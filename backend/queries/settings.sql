-- name: GetSetting :one
SELECT key, value, updated_by, updated_at FROM settings WHERE key = $1;

-- name: UpsertSetting :one
INSERT INTO settings (key, value, updated_by, updated_at) VALUES ($1, $2, $3, now())
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = now()
RETURNING key, value, updated_by, updated_at;
