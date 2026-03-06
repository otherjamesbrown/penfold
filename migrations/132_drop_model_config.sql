-- +goose Up
DROP TABLE IF EXISTS model_config;

-- +goose Down
-- Recreate model_config if rolling back (basic structure only, data would need re-seeding)
CREATE TABLE IF NOT EXISTS model_config (
    id SERIAL PRIMARY KEY,
    tenant_id UUID,
    config_key VARCHAR(255) NOT NULL,
    config_value TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
