-- name: DeleteBarByID :execrows
DELETE FROM bar WHERE id = $1;

-- name: DeleteBarByIDAndName :execrows
DELETE FROM bar WHERE id = $1 AND name = $2;

-- name: CountBars :one
SELECT count(*) FROM bar;
