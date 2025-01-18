-- name: CreateUser :one
INSERT INTO users (id, created_at, updated_at, email, hashed_password)
VALUES (
    gen_random_uuid(),
    NOW(),
    NOW(),
    $1, 
    $2
)
RETURNING *;

-- name: GetUserFromEmail :one
SELECT * FROM users
WHERE email = $1;

-- name: Reset :exec
DELETE FROM users;

-- name: UpdatePasswordAndEmail :one
UPDATE users
SET hashed_password = $1,
email = $2
WHERE id = $3
RETURNING *;

-- name: UpgradeUserToRed :one
UPDATE users
SET is_chirpy_red = True
WHERE id = $1
RETURNING *;