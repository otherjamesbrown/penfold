CREATE TABLE IF NOT EXISTS gmail_sync_state (
    tenant_id VARCHAR(255) NOT NULL,
    sync_id VARCHAR(255) NOT NULL,
    sync_type VARCHAR(50) NOT NULL,
    status VARCHAR(50) NOT NULL,
    history_id BIGINT DEFAULT 0,
    processed_count INT DEFAULT 0,
    total_count INT DEFAULT 0,
    error_count INT DEFAULT 0,
    next_page_token TEXT,
    labels TEXT[],
    query TEXT,
    errors JSONB DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, sync_id)
);
