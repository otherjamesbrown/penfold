-- 049_conversations.sql
-- Creates conversations, conversation_items, and conversation_participants tables.
-- Specification: SPEC-10 (Conversation objects + auto-linking from threads)
-- Shard: pf-c6bd69
--
-- Purpose: Provide a unified conversation object that aggregates related content
-- items (emails, meetings, documents) around a topic, with participants tracked.
-- Initially backfilled from email_threads.

-- +goose Up

-- =====================================================
-- Conversations Table
-- =====================================================

CREATE TABLE conversations (
    id TEXT PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    topic TEXT NOT NULL,
    thread_key TEXT,
    first_seen TIMESTAMPTZ,
    last_seen TIMESTAMPTZ,
    participant_count INT DEFAULT 0,
    item_count INT DEFAULT 0,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_conversations_tenant_thread_key ON conversations(tenant_id, thread_key) WHERE thread_key IS NOT NULL;
CREATE INDEX idx_conversations_tenant_id ON conversations(tenant_id);
CREATE INDEX idx_conversations_topic ON conversations(tenant_id, topic);
CREATE INDEX idx_conversations_last_seen ON conversations(tenant_id, last_seen DESC);

COMMENT ON TABLE conversations IS 'Unified conversation objects aggregating related content items around a topic';
COMMENT ON COLUMN conversations.thread_key IS 'Optional link to source thread (e.g., email root_message_id)';
COMMENT ON COLUMN conversations.topic IS 'Conversation subject/topic (normalized, stripped of RE:/FW: prefixes)';

-- =====================================================
-- Conversation Items Table
-- =====================================================

CREATE TABLE conversation_items (
    conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    content_id TEXT NOT NULL,
    source_id INT,
    added_at TIMESTAMPTZ DEFAULT NOW(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    PRIMARY KEY (conversation_id, content_id)
);

CREATE INDEX idx_conversation_items_conversation_id ON conversation_items(conversation_id);
CREATE INDEX idx_conversation_items_content_id ON conversation_items(content_id);
CREATE INDEX idx_conversation_items_source_id ON conversation_items(source_id) WHERE source_id IS NOT NULL;
CREATE INDEX idx_conversation_items_tenant_id ON conversation_items(tenant_id);

COMMENT ON TABLE conversation_items IS 'Junction table linking content items to conversations';
COMMENT ON COLUMN conversation_items.content_id IS 'Reference to content (e.g., em-xxxxx, mt-xxxxx)';
COMMENT ON COLUMN conversation_items.source_id IS 'Optional: source ID for legacy compatibility';

-- =====================================================
-- Conversation Participants Table
-- =====================================================

CREATE TABLE conversation_participants (
    id BIGSERIAL PRIMARY KEY,
    conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    name TEXT,
    address TEXT,
    tenant_id UUID NOT NULL REFERENCES tenants(id)
);

CREATE UNIQUE INDEX idx_conversation_participants_unique ON conversation_participants(conversation_id, COALESCE(address, name));

CREATE INDEX idx_conversation_participants_conversation_id ON conversation_participants(conversation_id);
CREATE INDEX idx_conversation_participants_address ON conversation_participants(address) WHERE address IS NOT NULL;
CREATE INDEX idx_conversation_participants_name ON conversation_participants(name) WHERE name IS NOT NULL;
CREATE INDEX idx_conversation_participants_tenant_id ON conversation_participants(tenant_id);

COMMENT ON TABLE conversation_participants IS 'Participants in a conversation (email addresses and/or names)';
COMMENT ON COLUMN conversation_participants.name IS 'Participant display name (at least one of name or address must be non-null)';
COMMENT ON COLUMN conversation_participants.address IS 'Participant email address (at least one of name or address must be non-null)';

-- =====================================================
-- Backfill from email_threads
-- =====================================================

-- Populate conversations from existing email_threads
-- For each thread, create a conversation with normalized topic
INSERT INTO conversations (
    id,
    tenant_id,
    topic,
    thread_key,
    first_seen,
    last_seen,
    participant_count,
    item_count,
    metadata,
    created_at,
    updated_at
)
SELECT
    'conv-thread-' || et.id AS id,
    et.tenant_id::uuid,
    -- Normalize subject: strip Re:/RE:/Fwd:/FW:/FWD: prefixes (case-insensitive, repeated)
    regexp_replace(
        et.subject,
        '^(\s*(re|fwd?)\s*:\s*)+',
        '',
        'gi'
    ) AS topic,
    et.root_message_id AS thread_key,
    et.first_message_at AS first_seen,
    et.last_message_at AS last_seen,
    0 AS participant_count, -- Will be updated after participant backfill
    et.message_count AS item_count,
    '{}'::jsonb AS metadata,
    et.created_at,
    et.updated_at
FROM email_threads et
ON CONFLICT (id) DO NOTHING;

-- Populate conversation_items from thread_messages
-- Join through thread_messages to get source_id and content_id
INSERT INTO conversation_items (
    conversation_id,
    content_id,
    source_id,
    added_at,
    tenant_id
)
SELECT DISTINCT
    'conv-thread-' || tm.thread_id AS conversation_id,
    COALESCE(s.content_id, 'src-' || s.id::text) AS content_id, -- Use content_id if available, fallback to src-<id>
    s.id::int AS source_id,
    tm.added_at,
    et.tenant_id::uuid
FROM thread_messages tm
JOIN email_threads et ON tm.thread_id = et.id
JOIN sources s ON tm.source_id = s.id
ON CONFLICT (conversation_id, content_id) DO NOTHING;

-- Populate conversation_participants from email headers
-- Extract from/to/cc addresses from sources ingestion_metadata
WITH from_participants AS (
    SELECT DISTINCT
        'conv-thread-' || tm.thread_id AS conversation_id,
        et.tenant_id::uuid AS tenant_id,
        NULLIF(trim(COALESCE(s.ingestion_metadata->>'from_name', '')), '') AS name,
        NULLIF(trim(COALESCE(s.ingestion_metadata->>'from_email', '')), '') AS address
    FROM thread_messages tm
    JOIN email_threads et ON tm.thread_id = et.id
    JOIN sources s ON tm.source_id = s.id
    WHERE trim(COALESCE(s.ingestion_metadata->>'from_name', '')) != ''
       OR trim(COALESCE(s.ingestion_metadata->>'from_email', '')) != ''
),
to_participants AS (
    SELECT DISTINCT
        'conv-thread-' || tm.thread_id AS conversation_id,
        et.tenant_id::uuid AS tenant_id,
        NULLIF(trim(addr->>'name'), '') AS name,
        NULLIF(trim(addr->>'email'), '') AS address
    FROM thread_messages tm
    JOIN email_threads et ON tm.thread_id = et.id
    JOIN sources s ON tm.source_id = s.id
    CROSS JOIN LATERAL jsonb_array_elements(COALESCE(s.ingestion_metadata->'to', '[]'::jsonb)) AS addr
    WHERE NULLIF(trim(addr->>'name'), '') IS NOT NULL
       OR NULLIF(trim(addr->>'email'), '') IS NOT NULL
),
cc_participants AS (
    SELECT DISTINCT
        'conv-thread-' || tm.thread_id AS conversation_id,
        et.tenant_id::uuid AS tenant_id,
        NULLIF(trim(addr->>'name'), '') AS name,
        NULLIF(trim(addr->>'email'), '') AS address
    FROM thread_messages tm
    JOIN email_threads et ON tm.thread_id = et.id
    JOIN sources s ON tm.source_id = s.id
    CROSS JOIN LATERAL jsonb_array_elements(COALESCE(s.ingestion_metadata->'cc', '[]'::jsonb)) AS addr
    WHERE NULLIF(trim(addr->>'name'), '') IS NOT NULL
       OR NULLIF(trim(addr->>'email'), '') IS NOT NULL
),
all_participants AS (
    SELECT * FROM from_participants
    UNION
    SELECT * FROM to_participants
    UNION
    SELECT * FROM cc_participants
)
INSERT INTO conversation_participants (
    conversation_id,
    name,
    address,
    tenant_id
)
SELECT DISTINCT
    conversation_id,
    name,
    address,
    tenant_id
FROM all_participants
WHERE name IS NOT NULL OR address IS NOT NULL
ON CONFLICT (conversation_id, (COALESCE(address, name))) DO NOTHING;

-- Update participant counts in conversations
UPDATE conversations c
SET participant_count = (
    SELECT COUNT(*)
    FROM conversation_participants cp
    WHERE cp.conversation_id = c.id
)
WHERE EXISTS (
    SELECT 1 FROM conversation_participants cp WHERE cp.conversation_id = c.id
);

-- +goose Down

DROP TABLE IF EXISTS conversation_participants;
DROP TABLE IF EXISTS conversation_items;
DROP TABLE IF EXISTS conversations;
