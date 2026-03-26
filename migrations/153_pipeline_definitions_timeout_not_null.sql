-- +goose Up
UPDATE pipeline_definitions SET timeout_seconds = 120 WHERE timeout_seconds IS NULL;
ALTER TABLE pipeline_definitions ALTER COLUMN timeout_seconds SET NOT NULL;

-- +goose Down
ALTER TABLE pipeline_definitions ALTER COLUMN timeout_seconds DROP NOT NULL;
