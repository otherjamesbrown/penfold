-- +goose Up
ALTER TABLE digests ALTER COLUMN project_id DROP NOT NULL;

-- +goose Down
-- Cannot re-add NOT NULL if NULL values exist
-- ALTER TABLE digests ALTER COLUMN project_id SET NOT NULL;
