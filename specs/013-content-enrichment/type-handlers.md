# Type-Specific Handlers

Part of [Content Enrichment Pipeline](spec.md)

---

## Overview

Type-specific handlers extract structured data based on content subtype. Each handler runs after common enrichment and before AI processing.

---

## Link Extraction & Enrichment

### Problem

Emails contain valuable references to external content:
- Google Docs/Sheets/Slides links (collaborative documents)
- Jira ticket URLs (project tracking state)
- Confluence/SharePoint links (knowledge base)
- Meeting recordings (Zoom, WebEx)
- File sharing links (Box, Dropbox, OneDrive)

These links represent **first-class content** that should be:
1. Extracted and catalogued
2. Enriched with metadata (if we have API access)
3. Linked bidirectionally to source emails
4. Potentially ingested as separate sources

### Link Categories

| Category | URL Pattern | Enrichment Possible | Value |
|----------|-------------|---------------------|-------|
| `google_doc` | `docs.google.com/document/*` | Yes (API) | High - collaborative content |
| `google_sheet` | `docs.google.com/spreadsheets/*` | Yes (API) | High - data/planning |
| `google_slides` | `docs.google.com/presentation/*` | Yes (API) | High - presentations |
| `jira_ticket` | `*.atlassian.net/browse/*`, `*/jira/*/browse/*` | Yes (API) | High - project state |
| `jira_board` | `*.atlassian.net/jira/*/board/*` | Limited | Medium - context |
| `confluence` | `*.atlassian.net/wiki/*` | Yes (API) | High - documentation |
| `webex_recording` | `*.webex.com/recordingservice/*` | Limited | Medium - meeting content |
| `zoom_recording` | `zoom.us/rec/*` | Limited | Medium - meeting content |
| `sharepoint` | `*.sharepoint.com/*` | Yes (Graph API) | High - enterprise docs |
| `github` | `github.com/*/*` | Yes (API) | Medium - code references |
| `generic_url` | `*` | Title only | Low - context |

### Link Data Model

```
┌─────────────────────────┐       ┌─────────────────────────┐
│   extracted_links       │       │   link_sources          │
├─────────────────────────┤       ├─────────────────────────┤
│ id                      │       │ link_id                 │
│ tenant_id               │       │ source_id               │
│ url                     │       │ context_snippet         │  ← Text around link
│ url_hash                │◄──────│ position_in_body        │
│ category                │       │ in_signature            │  ← Flag for signature links
│ first_seen_at           │       │ extracted_at            │
│ last_seen_at            │       └─────────────────────────┘
│ occurrence_count        │
└──────────┬──────────────┘
           │
┌──────────┴──────────────┐
│   link_enrichment       │
├─────────────────────────┤
│ link_id                 │
│ external_id             │  ← Google doc ID, Jira ticket key
│ title                   │
│ description             │  ← Summary/excerpt
│ owner_person_id         │  ← Resolved owner if known
│ last_modified           │
│ status                  │  ← For Jira: Open/In Progress/Done
│ metadata_json           │  ← Category-specific data
│ enriched_at             │
│ enrichment_error        │  ← If enrichment failed
└─────────────────────────┘
```

This allows:
- Same link appearing in multiple emails → one `extracted_links` row, multiple `link_sources` rows
- Querying "all emails that contain link X"
- Tracking how often a link is shared

### Jira Ticket Enrichment

When we see a Jira ticket reference (from notification OR embedded link):

```json
{
  "ticket_key": "OUT-697",
  "project": "OUT",
  "summary": "Launch new products for MTC customer 2026",
  "status": "In Progress",
  "status_category": "indeterminate",
  "assignee_person_id": "uuid",
  "reporter_person_id": "uuid",
  "priority": "High",
  "labels": ["mtc", "q1-2026"],
  "epic_key": "OUT-500",
  "last_updated": "timestamp",
  "comment_count": 12
}
```

**Jira Notification vs Link:**
- Notification email: Extract state *change* (what happened)
- Embedded link: Fetch current state (what it is now)
- Both link to same `jira_tickets` record

### Google Doc Enrichment

When we have Google Workspace API access:

```json
{
  "doc_id": "1abc...xyz",
  "title": "MTC Architecture Proposal",
  "mime_type": "application/vnd.google-apps.document",
  "owner_email": "jabrown@akamai.com",
  "owner_person_id": "uuid",
  "last_modified_by_person_id": "uuid",
  "last_modified": "timestamp",
  "shared_with": ["person_id1", "person_id2"],
  "word_count": 2500,
  "can_read_content": true
}
```

**If we can read content**: Queue for separate ingest as `document/google_doc` source.

### Link Edge Cases

| Edge Case | Handling |
|-----------|----------|
| URL shortener (bit.ly, t.co) | Follow redirect to get real URL, store both |
| Same doc linked in multiple emails | Single `extracted_links` record, multiple source_id associations |
| Link in email signature (every email) | Detect signature region, flag links as `in_signature=true` |
| Broken/expired link | Store as-is, mark `enrichment_error`, don't fail pipeline |
| Google Doc with restricted access | Store link, enrichment returns "access denied", still useful for correlation |
| Jira ticket that no longer exists | Store reference, mark as `deleted` or `not_found` |
| Private GitHub repo link | Store link, note access status in enrichment |

---

## Jira Handler

### Jira State Model

```
┌─────────────────────┐
│    jira_tickets     │
├─────────────────────┤
│ id                  │
│ tenant_id           │
│ ticket_key          │  ← "OUT-697"
│ project_key         │  ← "OUT"
│ summary             │
│ status              │  ← Current status
│ status_category     │  ← todo/indeterminate/done
│ assignee_person_id  │
│ reporter_person_id  │
│ priority            │
│ labels[]            │
│ epic_key            │
│ penfold_project_id  │  ← Linked internal project
│ first_seen_at       │
│ last_updated_at     │
└──────────┬──────────┘
           │
┌──────────┴──────────┐
│ jira_ticket_changes │
├─────────────────────┤
│ id                  │
│ ticket_id           │
│ source_id           │  ← Notification email
│ change_type         │  ← created, status_changed, assigned, commented
│ field_name          │  ← "status", "assignee"
│ from_value          │
│ to_value            │
│ changed_by_person_id│
│ changed_at          │
└─────────────────────┘
```

### Jira Notification Processing

```
Notification Email → Extract State Change → Update jira_tickets table
                                         → Link to source email
                                         → NO embedding

State Change Record:
{
  "ticket_key": "OUT-697",
  "change_type": "status_change",
  "from_status": "Open",
  "to_status": "In Progress",
  "changed_by_person_id": "uuid",
  "changed_at": "timestamp",
  "source_id": "uuid"  ← Link back to notification email
}
```

This means:
- "What changed on OUT-697 last week?" → Query `jira_ticket_changes` table
- "Show me discussions about OUT-697" → Query emails with `extracted_links` to that ticket
- No notification spam in vector search results

---

## Calendar Handler

### Meeting State Model

```
┌─────────────────────┐
│      meetings       │
├─────────────────────┤
│ id                  │
│ tenant_id           │
│ ical_uid            │  ← Unique calendar event ID
│ organizer_person_id │
│ title               │
│ description         │
│ location            │
│ video_url           │  ← WebEx, Zoom link
│ start_time          │
│ end_time            │
│ recurrence_rule     │  ← RRULE if recurring
│ status              │  ← active, cancelled, updated
│ created_at          │
│ updated_at          │
└──────────┬──────────┘
           │
┌──────────┴──────────┐
│  meeting_attendees  │
├─────────────────────┤
│ meeting_id          │
│ person_id           │
│ response_status     │  ← accepted/declined/tentative/none
│ is_optional         │
│ updated_at          │
└─────────────────────┘
           │
┌──────────┴──────────┐
│   meeting_events    │
├─────────────────────┤
│ id                  │
│ meeting_id          │
│ source_id           │  ← Calendar email
│ event_type          │  ← invite_sent, cancelled, updated, response
│ details_json        │  ← Event-specific data
│ occurred_at         │
└─────────────────────┘
```

### Calendar Processing Flows

**Meeting Invite (calendar/invite)**
```
Email → Classify as calendar/invite
      → Extract iCal UID + meeting details
      → Resolve organizer + attendees to people
      → Create meetings record (status=active)
      → Create meeting_attendees records
      → Create meeting_event record (invite_sent)
      → Link to source email
      → NO embedding (metadata only)
      → NO summary
```

**Meeting Cancellation (calendar/cancellation)**
```
Email → Classify as calendar/cancellation
      → Extract meeting UID + attendees
      → Resolve attendees to people
      → Update meetings table (status=cancelled)
      → Create meeting_event record
      → Link to source email
      → NO embedding
      → NO summary
```

---

## Attachment Handler

### Problem

Email attachments contain valuable content:
- Spreadsheets with data/plans
- PDFs with reports/contracts
- Images with diagrams/screenshots
- Office documents with proposals

Currently attachments are noted but not processed.

### Attachment Categories

| Category | Extensions | Processing |
|----------|------------|------------|
| `document` | .pdf, .doc, .docx | Extract text, summarize |
| `spreadsheet` | .xls, .xlsx, .csv | Extract structure, key data |
| `presentation` | .ppt, .pptx | Extract text, slide count |
| `image` | .png, .jpg, .gif | OCR if text-heavy, describe |
| `archive` | .zip, .tar | List contents, flag for review |
| `calendar` | .ics | Parse as calendar event |
| `code` | .py, .go, .js | Syntax highlight, no AI |
| `other` | * | Store metadata only |

### Attachment Data Model

```
┌─────────────────────┐
│  email_attachments  │
├─────────────────────┤
│ id                  │
│ tenant_id           │
│ source_id           │  ← Parent email
│ filename            │
│ content_type        │  ← MIME type
│ size_bytes          │
│ category            │  ← document, spreadsheet, etc.
│ content_hash        │  ← SHA-256 for dedup
│ storage_path        │  ← Where file is stored
│ extracted_text      │  ← If text extraction succeeded
│ processing_status   │  ← pending/processed/failed/skipped
│ created_at          │
└──────────┬──────────┘
           │
┌──────────┴──────────┐
│attachment_enrichment│
├─────────────────────┤
│ attachment_id       │
│ page_count          │  ← For PDFs/presentations
│ word_count          │
│ has_tables          │  ← Spreadsheet-like content
│ has_images          │
│ language            │  ← Detected language
│ summary             │  ← AI-generated summary
│ embedding_id        │  ← If embedded separately
│ enriched_at         │
└─────────────────────┘
```

### Attachment Processing Rules

| Condition | Action |
|-----------|--------|
| Size > 10MB | Skip AI, store metadata only |
| PDF < 50 pages | Extract text, summarize, embed |
| PDF > 50 pages | Extract text, summarize, skip embed |
| Spreadsheet | Extract headers + sample rows |
| Image with text (OCR score > 0.7) | OCR + summarize |
| Image without text | Describe if < 1MB, skip if larger |
| Calendar .ics | Parse and link to meeting |
| Duplicate (same hash) | Link to existing, skip reprocessing |

### Attachment Edge Cases

| Edge Case | Handling |
|-----------|----------|
| Password-protected PDF | Store metadata, mark `extraction_error: password_protected` |
| Corrupted file | Store metadata, mark `extraction_error: corrupt` |
| Extremely large attachment (>50MB) | Store metadata only, skip download/storage |
| Image that's actually a scanned document | Apply OCR, categorize as `document_scan` |
| Attachment with same name but different content | Use content hash for identity, not filename |
| Attachment without extension | Use MIME type from email, magic bytes as fallback |
| Embedded image (inline in HTML) | Extract if >10KB, skip small icons/logos |
| Calendar attachment (.ics) | Parse as calendar event, link to meetings table |

---

## Thread Handler

### Thread Tracking

```
┌─────────────────────┐
│   email_threads     │
├─────────────────────┤
│ id                  │
│ tenant_id           │
│ root_message_id     │  ← Message-ID of first message
│ subject             │  ← Normalized (stripped RE:/FW:)
│ participant_ids[]   │  ← All people in thread
│ message_count       │
│ first_message_at    │
│ last_message_at     │
│ latest_source_id    │  ← Most recent message
│ project_id          │  ← If matched to project
│ thread_summary      │  ← AI-generated summary
│ summary_updated_at  │
│ created_at          │
│ updated_at          │
└─────────────────────┘
```

### Thread Detection

Via `In-Reply-To` and `References` headers:

```go
func (g *ThreadGrouper) Group(source *Source) *string {
    // 1. Check In-Reply-To header
    if source.InReplyTo != "" {
        if thread := g.repo.GetThreadByMessageID(source.InReplyTo); thread != nil {
            return &thread.ID
        }
    }

    // 2. Check References header
    for _, ref := range source.References {
        if thread := g.repo.GetThreadByMessageID(ref); thread != nil {
            return &thread.ID
        }
    }

    // 3. Fallback: subject matching (if headers missing)
    normalizedSubject := normalizeSubject(source.Subject)
    if thread := g.repo.GetThreadBySubject(normalizedSubject, source.Date); thread != nil {
        return &thread.ID
    }

    // 4. No thread found - this is a new standalone or thread root
    return nil
}
```

### Thread Edge Cases

| Edge Case | Handling |
|-----------|----------|
| Broken thread (missing In-Reply-To) | Fall back to subject matching with time window |
| Thread forked into two conversations | Create two threads, link to common ancestor |
| Very long thread (50+ messages) | Chunk summaries, provide "key moments" view |
| Thread with subject change mid-conversation | Track as same thread, note subject variations |
| Thread containing both discussion and notifications | Separate AI processing, unified thread view |

---

## Forward Handler

### Forward Processing

```
Email → Classify as email/forward
      → Extract original sender from quoted "From:" line
      → Parse forwarded content
      → Store forwarded_from_person_id
      → Full AI processing on combined content
```

```go
type ForwardedContent struct {
    OriginalFromPersonID string    `json:"original_from_person_id"`
    OriginalDate         time.Time `json:"original_date,omitempty"`
    OriginalSubject      string    `json:"original_subject,omitempty"`
    ForwardChain         int       `json:"forward_chain"` // How many times forwarded
}
```

---

## Google Notification Handler

### Processing

```
Email → Classify as notification/google
      → Extract doc ID + action (comment, mention)
      → Extract mentioned users
      → Resolve to people
      → Create/update extracted_links
      → Enrich via Google API (if available):
          → Doc title, owner, last modified
          → Optionally queue doc for ingest
      → NO embedding
```

### Extracted Data

```json
{
  "service": "docs|slides|sheets|drive",
  "document_id": "string",
  "action": "comment|mention|share|edit",
  "mentioned_person_ids": ["uuid"],
  "document_title": "string",
  "document_owner_person_id": "uuid"
}
```

---

## Functional Requirements

### Link Extraction

- **FR-400**: System MUST extract all URLs from email body text and HTML
- **FR-401**: System MUST categorize links by type (google_doc, jira_ticket, confluence, etc.)
- **FR-402**: System MUST deduplicate links by URL hash within and across emails
- **FR-403**: System MUST store context snippet (surrounding text) for each link
- **FR-404**: System MUST track link position in body (early links often more important)
- **FR-405**: System SHOULD enrich links with external metadata when API access available
- **FR-406**: System MUST resolve link owners/authors to person records when possible
- **FR-407**: System MUST support querying "all emails that reference document X"

### Jira Integration

- **FR-420**: System MUST maintain `jira_tickets` table with current ticket state
- **FR-421**: System MUST extract ticket state from Jira notification emails
- **FR-422**: System MUST track state changes over time in `jira_ticket_changes`
- **FR-423**: System MUST link Jira tickets to emails that reference them
- **FR-424**: System SHOULD fetch current ticket state via Jira API if configured
- **FR-425**: System MUST resolve Jira users (assignee, reporter) to person records
- **FR-426**: System MUST NOT embed Jira notification emails (metadata only)

### Meeting Tracking

- **FR-250**: System MUST maintain `meetings` table with canonical meeting state
- **FR-251**: System MUST link calendar emails to meeting records via iCalendar UID
- **FR-252**: System MUST track meeting status changes (invite → cancel → update)
- **FR-253**: System MUST resolve meeting attendees to person records
- **FR-254**: System MUST support querying meetings by participant, organizer, date range
- **FR-255**: System SHOULD extract meeting locations and video conferencing URLs

### Thread Tracking

- **FR-260**: System MUST maintain `email_threads` table grouping related messages
- **FR-261**: System MUST use In-Reply-To and References headers for thread membership
- **FR-262**: System MUST track all thread participants across all messages
- **FR-263**: System MUST normalize subject lines for display (strip Re:/Fwd: prefixes)
- **FR-264**: System SHOULD fall back to subject matching when headers are missing
- **FR-265**: System MUST support querying threads by participant, date range, keywords

### Attachment Processing

- **FR-440**: System MUST extract and store attachments from emails
- **FR-441**: System MUST categorize attachments by type (document, spreadsheet, image, etc.)
- **FR-442**: System MUST deduplicate attachments by content hash
- **FR-443**: System MUST extract text from PDF and Office documents
- **FR-444**: System SHOULD apply OCR to images with text content
- **FR-445**: System MUST respect size limits for AI processing (configurable, default 10MB)
- **FR-446**: System MUST link attachments bidirectionally to source emails
- **FR-447**: System SHOULD generate separate embeddings for large attachments
