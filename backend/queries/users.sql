-- name: ListUsers :many
SELECT id, name, email, role, active, created_at, updated_at
FROM users
WHERE (sqlc.narg('active')::boolean IS NULL OR active = sqlc.narg('active')::boolean)
ORDER BY name;

-- name: GetUser :one
SELECT id, name, email, role, active, created_at, updated_at
FROM users
WHERE id = $1;

-- name: CreateUser :one
INSERT INTO users (name, email, role)
VALUES ($1, $2, $3)
RETURNING id, name, email, role, active, created_at, updated_at;

-- name: UpdateUser :one
UPDATE users SET
    name       = COALESCE(sqlc.narg('name'), name),
    email      = CASE WHEN sqlc.arg('set_email')::boolean THEN sqlc.narg('email') ELSE email END,
    role       = COALESCE(sqlc.narg('role'), role),
    active     = COALESCE(sqlc.narg('active'), active),
    updated_at = now()
WHERE id = sqlc.arg('id')
RETURNING id, name, email, role, active, created_at, updated_at;
