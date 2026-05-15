-- name: GetSchemaVersion :one
select version from schema_versions ORDER BY version DESC LIMIT 1;