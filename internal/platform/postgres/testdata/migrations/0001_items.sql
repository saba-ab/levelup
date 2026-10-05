-- +goose Up
CREATE TABLE items (
    id   BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name TEXT NOT NULL
);

-- +goose Down
DROP TABLE items;
