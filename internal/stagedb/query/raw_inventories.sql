-- name: SetRawInventory :one
INSERT INTO raw_inventories(digest, bytes)
VALUES (?, ?)
RETURNING digest;
