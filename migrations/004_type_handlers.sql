-- =====================================================
-- Type Handler Tables Migration
-- Created: 2026-01-19
-- Description: Tables for links, Jira, meetings, threads, and attachments
-- Specification: specs/013-content-enrichment/type-handlers.md
-- =====================================================

-- =====================================================
-- Link Extraction Tables
-- =====================================================

-- Enum for link categories
DO $$ BEGIN
    CREATE TYPE link_category AS ENUM (
        'google_doc', 'google_sheet', 'google_slides', 'google_drive',
        'jira_ticket', 'jira_board', 'confluence',
        'webex_recording', 'zoom_recording',
        'sharepoint', 'onedrive',
        'github', 'gitlab', 'bitbucket',
        'slack', 'teams',
        'generic_url'
    );
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Extracted links table (deduplicated by URL hash)
CREATE TABLE IF NOT EXISTS extracted_links (
    id BIGSERIAL PRIMARY KEY,
    tenant_id VARCHAR(255) NOT NULL,

    -- Link identity
    url TEXT NOT NULL,
    url_hash VARCHAR(64) NOT NULL, -- SHA-256 hash for dedup

    -- Classification
    category link_category NOT NULL DEFAULT 'generic_url',
    external_id VARCHAR(255), -- Doc ID, ticket key, etc.

    -- Tracking
    first_seen_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    last_seen_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    occurrence_count INT DEFAULT 1 NOT NULL,

    -- Constraints
    CONSTRAINT extracted_links_url_not_empty CHECK (length(trim(url)) > 0),
    CONSTRAINT extracted_links_tenant_hash_unique UNIQUE (tenant_id, url_hash)
);

-- Indexes for extracted_links
CREATE INDEX IF NOT EXISTS idx_extracted_links_tenant_id ON extracted_links (tenant_id);
CREATE INDEX IF NOT EXISTS idx_extracted_links_url_hash ON extracted_links (url_hash);
CREATE INDEX IF NOT EXISTS idx_extracted_links_category ON extracted_links (tenant_id, category);
CREATE INDEX IF NOT EXISTS idx_extracted_links_external_id ON extracted_links (tenant_id, external_id) WHERE external_id IS NOT NULL;

-- Link sources junction table (tracks which sources contain which links)
CREATE TABLE IF NOT EXISTS link_sources (
    id BIGSERIAL PRIMARY KEY,
    link_id BIGINT NOT NULL REFERENCES extracted_links(id) ON DELETE CASCADE,
    source_id BIGINT NOT NULL, -- References sources table

    -- Context
    context_snippet TEXT, -- Text around the link (up to 200 chars)
    position_in_body INT, -- Character offset in body
    in_signature BOOLEAN DEFAULT FALSE, -- Flag for signature links
    anchor_text TEXT, -- Hyperlink text if available

    -- Timestamps
    extracted_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,

    -- Constraints
    CONSTRAINT link_sources_unique UNIQUE (link_id, source_id)
);

-- Indexes for link_sources
CREATE INDEX IF NOT EXISTS idx_link_sources_link_id ON link_sources (link_id);
CREATE INDEX IF NOT EXISTS idx_link_sources_source_id ON link_sources (source_id);
CREATE INDEX IF NOT EXISTS idx_link_sources_not_signature ON link_sources (link_id) WHERE in_signature = FALSE;

-- Link enrichment table (metadata fetched from external APIs)
CREATE TABLE IF NOT EXISTS link_enrichment (
    id BIGSERIAL PRIMARY KEY,
    link_id BIGINT NOT NULL REFERENCES extracted_links(id) ON DELETE CASCADE UNIQUE,

    -- Enriched metadata
    title TEXT,
    description TEXT,
    owner_person_id BIGINT REFERENCES people(id) ON DELETE SET NULL,
    last_modified TIMESTAMPTZ,
    status VARCHAR(100), -- For Jira: Open/In Progress/Done
    metadata_json JSONB DEFAULT '{}',

    -- Enrichment status
    enriched_at TIMESTAMPTZ,
    enrichment_error TEXT, -- If enrichment failed

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL
);

-- Indexes for link_enrichment
CREATE INDEX IF NOT EXISTS idx_link_enrichment_link_id ON link_enrichment (link_id);
CREATE INDEX IF NOT EXISTS idx_link_enrichment_owner ON link_enrichment (owner_person_id) WHERE owner_person_id IS NOT NULL;

-- =====================================================
-- Jira Integration Tables
-- =====================================================

-- Enum for Jira status categories
DO $$ BEGIN
    CREATE TYPE jira_status_category AS ENUM ('todo', 'indeterminate', 'done');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Enum for Jira change types
DO $$ BEGIN
    CREATE TYPE jira_change_type AS ENUM (
        'created', 'status_changed', 'assigned', 'commented',
        'priority_changed', 'label_added', 'label_removed',
        'epic_linked', 'resolved', 'reopened', 'other'
    );
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Jira tickets table (current state)
CREATE TABLE IF NOT EXISTS jira_tickets (
    id BIGSERIAL PRIMARY KEY,
    tenant_id VARCHAR(255) NOT NULL,

    -- Ticket identity
    ticket_key VARCHAR(50) NOT NULL, -- e.g., "OUT-697"
    project_key VARCHAR(20) NOT NULL, -- e.g., "OUT"

    -- Current state
    summary TEXT,
    status VARCHAR(100),
    status_category jira_status_category,
    priority VARCHAR(50),
    labels TEXT[] DEFAULT '{}',
    epic_key VARCHAR(50),

    -- People
    assignee_person_id BIGINT REFERENCES people(id) ON DELETE SET NULL,
    reporter_person_id BIGINT REFERENCES people(id) ON DELETE SET NULL,

    -- Penfold integration
    penfold_project_id BIGINT REFERENCES projects(id) ON DELETE SET NULL,

    -- Timestamps
    first_seen_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    last_updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,

    -- Constraints
    CONSTRAINT jira_tickets_key_not_empty CHECK (length(trim(ticket_key)) > 0),
    CONSTRAINT jira_tickets_tenant_key_unique UNIQUE (tenant_id, ticket_key)
);

-- Indexes for jira_tickets
CREATE INDEX IF NOT EXISTS idx_jira_tickets_tenant_id ON jira_tickets (tenant_id);
CREATE INDEX IF NOT EXISTS idx_jira_tickets_ticket_key ON jira_tickets (ticket_key);
CREATE INDEX IF NOT EXISTS idx_jira_tickets_project_key ON jira_tickets (tenant_id, project_key);
CREATE INDEX IF NOT EXISTS idx_jira_tickets_assignee ON jira_tickets (assignee_person_id) WHERE assignee_person_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_jira_tickets_status ON jira_tickets (tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_jira_tickets_labels ON jira_tickets USING GIN (labels);

-- Jira ticket changes table (state change history)
CREATE TABLE IF NOT EXISTS jira_ticket_changes (
    id BIGSERIAL PRIMARY KEY,
    ticket_id BIGINT NOT NULL REFERENCES jira_tickets(id) ON DELETE CASCADE,
    source_id BIGINT NOT NULL, -- Notification email that reported this change

    -- Change details
    change_type jira_change_type NOT NULL,
    field_name VARCHAR(100), -- "status", "assignee", etc.
    from_value TEXT,
    to_value TEXT,

    -- Who made the change
    changed_by_person_id BIGINT REFERENCES people(id) ON DELETE SET NULL,

    -- Timestamps
    changed_at TIMESTAMPTZ NOT NULL,
    recorded_at TIMESTAMPTZ DEFAULT NOW() NOT NULL
);

-- Indexes for jira_ticket_changes
CREATE INDEX IF NOT EXISTS idx_jira_ticket_changes_ticket_id ON jira_ticket_changes (ticket_id);
CREATE INDEX IF NOT EXISTS idx_jira_ticket_changes_source_id ON jira_ticket_changes (source_id);
CREATE INDEX IF NOT EXISTS idx_jira_ticket_changes_changed_at ON jira_ticket_changes (changed_at);
CREATE INDEX IF NOT EXISTS idx_jira_ticket_changes_change_type ON jira_ticket_changes (ticket_id, change_type);

-- =====================================================
-- Calendar/Meeting Tables
-- =====================================================

-- Enum for meeting status
DO $$ BEGIN
    CREATE TYPE meeting_status AS ENUM ('active', 'cancelled', 'updated');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Enum for meeting event types
DO $$ BEGIN
    CREATE TYPE meeting_event_type AS ENUM (
        'invite_sent', 'cancelled', 'updated', 'response_received',
        'reminder_sent', 'started', 'ended'
    );
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Enum for attendee response status
DO $$ BEGIN
    CREATE TYPE attendee_response AS ENUM ('accepted', 'declined', 'tentative', 'none');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Meetings table (canonical meeting state)
CREATE TABLE IF NOT EXISTS meetings (
    id BIGSERIAL PRIMARY KEY,
    tenant_id VARCHAR(255) NOT NULL,

    -- Meeting identity
    ical_uid VARCHAR(500) NOT NULL, -- Unique calendar event ID

    -- People
    organizer_person_id BIGINT REFERENCES people(id) ON DELETE SET NULL,

    -- Meeting details
    title TEXT NOT NULL,
    description TEXT,
    location TEXT,
    video_url TEXT, -- WebEx, Zoom link

    -- Timing
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ NOT NULL,
    recurrence_rule TEXT, -- RRULE if recurring

    -- Status
    status meeting_status NOT NULL DEFAULT 'active',

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,

    -- Constraints
    CONSTRAINT meetings_title_not_empty CHECK (length(trim(title)) > 0),
    CONSTRAINT meetings_tenant_uid_unique UNIQUE (tenant_id, ical_uid)
);

-- Indexes for meetings
CREATE INDEX IF NOT EXISTS idx_meetings_tenant_id ON meetings (tenant_id);
CREATE INDEX IF NOT EXISTS idx_meetings_ical_uid ON meetings (ical_uid);
CREATE INDEX IF NOT EXISTS idx_meetings_organizer ON meetings (organizer_person_id) WHERE organizer_person_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_meetings_start_time ON meetings (tenant_id, start_time);
CREATE INDEX IF NOT EXISTS idx_meetings_status ON meetings (tenant_id, status);

-- Meeting attendees table
CREATE TABLE IF NOT EXISTS meeting_attendees (
    id BIGSERIAL PRIMARY KEY,
    meeting_id BIGINT NOT NULL REFERENCES meetings(id) ON DELETE CASCADE,
    person_id BIGINT NOT NULL REFERENCES people(id) ON DELETE CASCADE,

    -- Attendance details
    response_status attendee_response NOT NULL DEFAULT 'none',
    is_optional BOOLEAN DEFAULT FALSE,

    -- Timestamps
    updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,

    -- Constraints
    CONSTRAINT meeting_attendees_unique UNIQUE (meeting_id, person_id)
);

-- Indexes for meeting_attendees
CREATE INDEX IF NOT EXISTS idx_meeting_attendees_meeting_id ON meeting_attendees (meeting_id);
CREATE INDEX IF NOT EXISTS idx_meeting_attendees_person_id ON meeting_attendees (person_id);
CREATE INDEX IF NOT EXISTS idx_meeting_attendees_response ON meeting_attendees (meeting_id, response_status);

-- Meeting events table (lifecycle tracking)
CREATE TABLE IF NOT EXISTS meeting_events (
    id BIGSERIAL PRIMARY KEY,
    meeting_id BIGINT NOT NULL REFERENCES meetings(id) ON DELETE CASCADE,
    source_id BIGINT NOT NULL, -- Calendar email that triggered this event

    -- Event details
    event_type meeting_event_type NOT NULL,
    details_json JSONB DEFAULT '{}',

    -- Timestamps
    occurred_at TIMESTAMPTZ NOT NULL,
    recorded_at TIMESTAMPTZ DEFAULT NOW() NOT NULL
);

-- Indexes for meeting_events
CREATE INDEX IF NOT EXISTS idx_meeting_events_meeting_id ON meeting_events (meeting_id);
CREATE INDEX IF NOT EXISTS idx_meeting_events_source_id ON meeting_events (source_id);
CREATE INDEX IF NOT EXISTS idx_meeting_events_occurred_at ON meeting_events (meeting_id, occurred_at);
CREATE INDEX IF NOT EXISTS idx_meeting_events_event_type ON meeting_events (meeting_id, event_type);

-- =====================================================
-- Email Thread Tables
-- =====================================================

-- Email threads table
CREATE TABLE IF NOT EXISTS email_threads (
    id BIGSERIAL PRIMARY KEY,
    tenant_id VARCHAR(255) NOT NULL,

    -- Thread identity
    root_message_id VARCHAR(500) NOT NULL, -- Message-ID of first message

    -- Thread metadata
    subject TEXT NOT NULL, -- Normalized (stripped RE:/FW:)
    participant_ids BIGINT[] DEFAULT '{}', -- All people in thread

    -- Statistics
    message_count INT DEFAULT 1 NOT NULL,

    -- Timing
    first_message_at TIMESTAMPTZ NOT NULL,
    last_message_at TIMESTAMPTZ NOT NULL,

    -- Latest message reference
    latest_source_id BIGINT NOT NULL, -- Most recent message

    -- Project linking
    project_id BIGINT REFERENCES projects(id) ON DELETE SET NULL,

    -- AI processing
    thread_summary TEXT,
    summary_updated_at TIMESTAMPTZ,

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,

    -- Constraints
    CONSTRAINT email_threads_subject_not_empty CHECK (length(trim(subject)) > 0),
    CONSTRAINT email_threads_tenant_root_unique UNIQUE (tenant_id, root_message_id)
);

-- Indexes for email_threads
CREATE INDEX IF NOT EXISTS idx_email_threads_tenant_id ON email_threads (tenant_id);
CREATE INDEX IF NOT EXISTS idx_email_threads_root_message_id ON email_threads (root_message_id);
CREATE INDEX IF NOT EXISTS idx_email_threads_subject ON email_threads (tenant_id, subject);
CREATE INDEX IF NOT EXISTS idx_email_threads_last_message ON email_threads (tenant_id, last_message_at DESC);
CREATE INDEX IF NOT EXISTS idx_email_threads_participants ON email_threads USING GIN (participant_ids);
CREATE INDEX IF NOT EXISTS idx_email_threads_project ON email_threads (project_id) WHERE project_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_email_threads_latest_source ON email_threads (latest_source_id);

-- Thread message membership (tracks which sources belong to which threads)
CREATE TABLE IF NOT EXISTS thread_messages (
    id BIGSERIAL PRIMARY KEY,
    thread_id BIGINT NOT NULL REFERENCES email_threads(id) ON DELETE CASCADE,
    source_id BIGINT NOT NULL, -- References sources table

    -- Message identity
    message_id VARCHAR(500) NOT NULL, -- Message-ID header

    -- Position in thread
    position_in_thread INT NOT NULL, -- 1, 2, 3, etc.
    is_reply BOOLEAN DEFAULT FALSE,
    reply_to_message_id VARCHAR(500), -- In-Reply-To header value

    -- Timestamps
    message_date TIMESTAMPTZ NOT NULL,
    added_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,

    -- Constraints
    CONSTRAINT thread_messages_unique UNIQUE (thread_id, source_id)
);

-- Indexes for thread_messages
CREATE INDEX IF NOT EXISTS idx_thread_messages_thread_id ON thread_messages (thread_id);
CREATE INDEX IF NOT EXISTS idx_thread_messages_source_id ON thread_messages (source_id);
CREATE INDEX IF NOT EXISTS idx_thread_messages_message_id ON thread_messages (message_id);
CREATE INDEX IF NOT EXISTS idx_thread_messages_date ON thread_messages (thread_id, message_date);

-- =====================================================
-- Attachment Tables
-- =====================================================

-- Enum for attachment categories
DO $$ BEGIN
    CREATE TYPE attachment_category AS ENUM (
        'document', 'spreadsheet', 'presentation', 'image',
        'archive', 'calendar', 'code', 'other'
    );
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Enum for attachment processing status
DO $$ BEGIN
    CREATE TYPE attachment_processing_status AS ENUM (
        'pending', 'processing', 'processed', 'failed', 'skipped'
    );
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Email attachments table
CREATE TABLE IF NOT EXISTS email_attachments (
    id BIGSERIAL PRIMARY KEY,
    tenant_id VARCHAR(255) NOT NULL,
    source_id BIGINT NOT NULL, -- Parent email

    -- File identity
    filename TEXT NOT NULL,
    content_type VARCHAR(255), -- MIME type
    size_bytes BIGINT NOT NULL,
    content_hash VARCHAR(64) NOT NULL, -- SHA-256 for dedup

    -- Classification
    category attachment_category NOT NULL DEFAULT 'other',

    -- Storage
    storage_path TEXT, -- Where file is stored

    -- Processing
    extracted_text TEXT, -- If text extraction succeeded
    processing_status attachment_processing_status NOT NULL DEFAULT 'pending',
    processing_error TEXT, -- If processing failed

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,

    -- Constraints
    CONSTRAINT email_attachments_filename_not_empty CHECK (length(trim(filename)) > 0)
);

-- Indexes for email_attachments
CREATE INDEX IF NOT EXISTS idx_email_attachments_tenant_id ON email_attachments (tenant_id);
CREATE INDEX IF NOT EXISTS idx_email_attachments_source_id ON email_attachments (source_id);
CREATE INDEX IF NOT EXISTS idx_email_attachments_content_hash ON email_attachments (tenant_id, content_hash);
CREATE INDEX IF NOT EXISTS idx_email_attachments_category ON email_attachments (tenant_id, category);
CREATE INDEX IF NOT EXISTS idx_email_attachments_processing_status ON email_attachments (tenant_id, processing_status);

-- Attachment enrichment table
CREATE TABLE IF NOT EXISTS attachment_enrichment (
    id BIGSERIAL PRIMARY KEY,
    attachment_id BIGINT NOT NULL REFERENCES email_attachments(id) ON DELETE CASCADE UNIQUE,

    -- Document metadata
    page_count INT, -- For PDFs/presentations
    word_count INT,
    has_tables BOOLEAN DEFAULT FALSE,
    has_images BOOLEAN DEFAULT FALSE,
    language VARCHAR(10), -- Detected language (ISO 639-1)

    -- AI processing
    summary TEXT, -- AI-generated summary
    embedding_id BIGINT, -- If embedded separately (references embeddings table)

    -- Timestamps
    enriched_at TIMESTAMPTZ DEFAULT NOW() NOT NULL
);

-- Indexes for attachment_enrichment
CREATE INDEX IF NOT EXISTS idx_attachment_enrichment_attachment_id ON attachment_enrichment (attachment_id);

-- =====================================================
-- Comments
-- =====================================================

COMMENT ON TABLE extracted_links IS 'Unique links extracted from content, deduplicated by URL hash';
COMMENT ON COLUMN extracted_links.url_hash IS 'SHA-256 hash of URL for efficient deduplication';
COMMENT ON COLUMN extracted_links.external_id IS 'Service-specific ID (Google doc ID, Jira ticket key, etc.)';

COMMENT ON TABLE link_sources IS 'Junction table tracking which sources contain which links';
COMMENT ON COLUMN link_sources.in_signature IS 'True if link appears in email signature (lower signal value)';

COMMENT ON TABLE link_enrichment IS 'Metadata fetched from external APIs for enriched links';

COMMENT ON TABLE jira_tickets IS 'Current state of Jira tickets extracted from notifications';
COMMENT ON COLUMN jira_tickets.status_category IS 'Simplified status: todo, indeterminate (in progress), done';
COMMENT ON COLUMN jira_tickets.penfold_project_id IS 'Link to internal Penfold project for unified project tracking';

COMMENT ON TABLE jira_ticket_changes IS 'State change history for Jira tickets from notification emails';
COMMENT ON COLUMN jira_ticket_changes.source_id IS 'The notification email that reported this change';

COMMENT ON TABLE meetings IS 'Canonical meeting records from calendar invites/updates';
COMMENT ON COLUMN meetings.ical_uid IS 'Unique iCalendar UID for matching invite/cancel/update events';
COMMENT ON COLUMN meetings.recurrence_rule IS 'iCalendar RRULE for recurring meetings';

COMMENT ON TABLE meeting_attendees IS 'Meeting attendance with response status';
COMMENT ON TABLE meeting_events IS 'Lifecycle events for meetings (invite, cancel, update, response)';

COMMENT ON TABLE email_threads IS 'Thread grouping for related email messages';
COMMENT ON COLUMN email_threads.root_message_id IS 'Message-ID of the first message in the thread';
COMMENT ON COLUMN email_threads.participant_ids IS 'Array of all person_ids who participated in this thread';

COMMENT ON TABLE thread_messages IS 'Tracks which source messages belong to which threads';

COMMENT ON TABLE email_attachments IS 'Email attachments with metadata and optional extracted text';
COMMENT ON COLUMN email_attachments.content_hash IS 'SHA-256 hash for deduplication of identical files';

COMMENT ON TABLE attachment_enrichment IS 'Enriched metadata for processed attachments';
