-- +goose Up
CREATE TABLE chirps(
    id          UUID        PRIMARY KEY,
    created_at  TIMESTAMP   NOT NULL,
    updated_at  TIMESTAMP   NOT NULL,
    body        TEXT        NOT NULL,
    user_id     UUID        REFERENCES users
                            ON DELETE CASCADE,
    FOREIGN KEY(user_id)
    REFERENCES users(id)
);

-- +goose Down
DROP TABLE chirps;