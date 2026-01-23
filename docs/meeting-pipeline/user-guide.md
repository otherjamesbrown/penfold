# Meeting Pipeline User Guide

**Version**: 2.0.0
**Implementation**: Go
**Target Audience**: Developers and system operators

## Overview

This guide walks you through ingesting meeting transcripts, resolving participants, extracting mentions, and processing meetings through the content enrichment pipeline.

## Getting Started

### Prerequisites

- Go 1.22+ installed
- PostgreSQL 16+ running with pgvector extension
- Penfold CLI (`penf`) built and available
- Database connection configured in `~/.penf/config.yaml`

### Quick Start

```bash
# Ingest meeting transcripts from a directory
penf ingest meeting ~/meetings/ --source "project-x"

# Resolve participants to known people
penf ingest meeting resolve

# Extract mentions of people from transcripts
penf ingest meeting mentions
```

## Supported File Formats

### WebVTT Transcripts (.vtt)

Standard format from Webex, Zoom, and other platforms:

```
WEBVTT

1 "John Smith" (123)
00:00:05.579 --> 00:00:10.858
Let's start with the agenda for today.

2 "Jane Doe"
00:00:11.000 --> 00:00:15.500
I have a few items to add to the discussion.
```

### Plain Text Transcripts (.txt)

Transcript format with timestamp, speaker, and text:

```
0:11 : John Smith : Let's start with the agenda for today.
0:15 : Jane Doe (she/her) : I have a few items to add to the discussion.
12:30 : John Smith : Let's wrap up with action items.
```

**Filename patterns recognized**:
- `Transcript_Meeting Name_20260123.txt`
- `Meeting Name-20260123 1430-1.vtt`

### Chat Logs (.txt)

Chat message format with date, time, speaker, and message:

```
2026-01-23 09:07 : John Smith : Hello everyone
2026-01-23 09:08 : Jane Doe : Check out <a href="https://example.com">this link</a>
-----> 2026-01-23 09:10 : Meeting Bot : Recording started
```

**Filename patterns recognized**:
- `Chat messages_Meeting Name_20260123.txt`
- `Chat_Meeting Name_20260123.txt`

### Video/Audio Files (metadata only)

The pipeline recognizes video and audio files but only stores metadata:
- **Video**: MP4, WebM, MOV, AVI
- **Audio**: M4A, MP3, WAV

## Ingesting Meetings

### Single File Ingestion

```bash
# Ingest a single VTT transcript
penf ingest meeting ./meeting.vtt --source "weekly-sync"

# Ingest a single TXT transcript
penf ingest meeting ./Transcript_Meeting_20260123.txt --source "archive"
```

### Directory Ingestion

The scanner automatically groups related files:

```
MeetingFolder/
  Weekly Sync-20260123 0900-1.vtt    # Transcript
  Chat messages_Weekly Sync_20260123.txt  # Chat log
  Weekly Sync-20260123 0900-1.mp4    # Video (metadata only)
```

```bash
# Ingest the directory as a single meeting
penf ingest meeting ./MeetingFolder/ --source "weekly-sync"
```

### Batch Ingestion

Scan a parent directory with multiple meeting subdirectories:

```
meetings/
  Project Alpha - 01152026/
    transcript.vtt
    chat.txt
  Project Beta - 01162026/
    transcript.vtt
```

```bash
# Ingest all meetings in the directory
penf ingest meeting ~/meetings/ --source "archive-2026"
```

### Dry Run Mode

Preview what would be imported without making changes:

```bash
penf ingest meeting ~/meetings/ --source "test" --dry-run
```

Output:
```
Meeting Ingest: /Users/james/meetings
  Source:      test
  Platform:    webex
  Tenant:      default
  Mode:        DRY RUN (no changes will be made)
  Path type:   directory

Scanning for meetings...
Found 3 meeting(s)

1. Weekly Sync (2026-01-23)
   Transcript: /Users/james/meetings/weekly/transcript.vtt
   Chat: /Users/james/meetings/weekly/chat.txt
2. Project Review (2026-01-22)
   Transcript: /Users/james/meetings/review/transcript.vtt
3. Planning Session (2026-01-21)
   Transcript: /Users/james/meetings/planning/transcript.txt

Dry run complete. No changes made.
```

### Platform Selection

Specify the meeting platform for metadata:

```bash
penf ingest meeting ./meeting.vtt --source "sync" --platform teams
```

Supported platforms:
- `webex` (default)
- `teams`
- `zoom`
- `google_meet`

## Participant Resolution

After ingesting meetings, resolve participant display names to known people in the database.

### How Resolution Works

1. **Canonical Match**: Exact match against `people.canonical_name`
2. **Alias Match**: Match against `people.aliases` array
3. **Name Normalization**: Strips pronouns like `(she/her)`, `(he/him)`

### Running Resolution

```bash
# Resolve all meetings
penf ingest meeting resolve

# Resolve meetings from a specific source
penf ingest meeting resolve --source "weekly-sync"
```

### Output

```
Resolving Meeting Participants
  Tenant: 00000001-0000-0000-0000-000000000001

Loading people from database...
  Found 42 people

Loading meetings...
  [1] Weekly Sync: 5/6 matched
  [2] Project Review: 4/4 matched
  [3] Planning Session: 3/5 matched

Resolution Complete
==================================================
  Meetings:     3
  Participants: 15
  Matched:      12
  Unmatched:    3
  Match Rate:   80.0%
```

### Improving Match Rates

1. **Add aliases** for people with multiple name variations:
   ```sql
   UPDATE people
   SET aliases = array_append(aliases, 'Johnny')
   WHERE canonical_name = 'John Smith';
   ```

2. **Handle pronouns**: The resolver automatically strips common pronoun patterns

3. **Review unmatched**: Query for unmatched participants:
   ```sql
   SELECT DISTINCT display_name
   FROM meeting_participants
   WHERE person_id IS NULL;
   ```

## Mention Extraction

Extract mentions of people discussed in meetings (distinct from attendees who spoke).

### How Extraction Works

1. Loads all people with their canonical names and aliases
2. Searches transcript text using word-boundary regex patterns
3. Excludes attendees (people who spoke) to avoid false positives
4. Records mention count and context snippet

### Running Extraction

```bash
penf ingest meeting mentions
```

### Output

```
Extracting Meeting Mentions
  Tenant: 00000001-0000-0000-0000-000000000001

Loading people from database...
  Found 42 people

Processing meeting transcripts...
  [1] Weekly Sync: 2 mentions (Alice Johnson, Bob Smith)
  [3] Planning Session: 1 mentions (Carol Williams)

Mention Extraction Complete
==================================================
  Meetings with mentions: 2
  Total mentions:         3
```

### Querying Mentions

Find meetings where a specific person was mentioned:

```sql
SELECT m.title, m.meeting_date, mm.mention_count, mm.context
FROM meeting_mentions mm
JOIN meetings m ON mm.meeting_id = m.id
JOIN people p ON mm.person_id = p.id
WHERE p.canonical_name = 'Alice Johnson'
ORDER BY m.meeting_date DESC;
```

## Acronym Detection

The pipeline automatically detects unknown acronyms and queues them for glossary review.

### How Detection Works

1. Scans transcript text for uppercase patterns (2-10 characters)
2. Filters out common words (API, URL, CEO, etc.)
3. Filters out terms already in the glossary
4. Queues remaining acronyms to the review queue

### Processing Detected Acronyms

Use the batch processing workflow:

```bash
# Get context for acronym review
penf process acronyms context --output json

# Batch resolve acronyms
penf process acronyms batch-resolve '{"resolutions":[...],"dismissals":[...]}'
```

Or review individually:

```bash
# List pending acronym questions
penf review list --type acronym

# Resolve an acronym
penf review resolve <question-id> --definition "Explanation of the term"

# Dismiss an acronym (not worth adding)
penf review dismiss <question-id> --reason "Common word"
```

## Content Enrichment

After ingestion, meetings are processed through the Temporal content workflow for:

1. **Embedding Generation**: Vector embeddings for semantic search
2. **Summary Generation**: AI-generated meeting summaries
3. **Entity Extraction**: Identify organizations, products, topics
4. **Topic Extraction**: Categorize discussion themes
5. **Mention Resolution**: Link references to known entities

### Triggering Enrichment

Content is automatically queued for enrichment on ingestion. Monitor progress:

```bash
# Check workflow status
penf workflow status

# View pending content
penf pipeline list --status pending
```

### Manual Enrichment

Trigger enrichment for specific content:

```bash
# Enrich a specific source
penf pipeline enrich --source-id 123

# Re-process all meeting sources
penf pipeline enrich --type meeting_transcript
```

## Database Tables

### meetings

Stores meeting metadata:

| Column | Type | Description |
|--------|------|-------------|
| id | BIGSERIAL | Primary key |
| tenant_id | UUID | Tenant identifier |
| title | TEXT | Meeting title |
| meeting_date | TIMESTAMPTZ | When the meeting occurred |
| platform | TEXT | webex, teams, zoom, google_meet |
| duration_seconds | INTEGER | Meeting duration |
| participants | JSONB | Array of participant names |
| source_tag | TEXT | User-provided tag |
| has_transcript | BOOLEAN | Transcript available |
| has_chat | BOOLEAN | Chat log available |

### sources

Stores transcript and chat content:

| Column | Type | Description |
|--------|------|-------------|
| id | BIGSERIAL | Primary key |
| meeting_id | BIGINT | FK to meetings |
| source_system | TEXT | meeting_transcript, meeting_chat |
| raw_content | TEXT | Full text content |
| content_hash | TEXT | SHA256 hash |
| processing_status | TEXT | pending, processing, completed |

### meeting_participants

Stores participant resolution:

| Column | Type | Description |
|--------|------|-------------|
| meeting_id | BIGINT | FK to meetings |
| person_id | BIGINT | FK to people (nullable) |
| display_name | TEXT | Original display name |
| match_type | TEXT | exact, alias, fuzzy |
| confidence | FLOAT | Match confidence score |

### meeting_mentions

Stores people mentioned in content:

| Column | Type | Description |
|--------|------|-------------|
| meeting_id | BIGINT | FK to meetings |
| person_id | BIGINT | FK to people |
| matched_text | TEXT | Text that matched |
| context | TEXT | Surrounding text snippet |
| mention_count | INTEGER | Number of occurrences |

## Troubleshooting

### No meetings found

Check file naming patterns:
- VTT files must have `.vtt` extension
- TXT transcripts should start with `Transcript_` or match timestamp pattern
- Chat logs should start with `Chat messages_` or `Chat_`

### Low participant match rate

1. Verify people exist in the database
2. Add aliases for name variations
3. Check for typos in canonical names

### Missing acronyms in review queue

Acronyms are only queued if:
- They appear at least once (configurable)
- They are not in the common words list
- They are not already in the glossary

### Enrichment not starting

Check Temporal connection:
```bash
penf health --component temporal
```

Check worker status:
```bash
penf workflow workers
```

## Best Practices

### Organizing Meeting Files

```
meetings/
  2026/
    01-january/
      weekly-sync-20260106/
        transcript.vtt
        chat.txt
      project-review-20260108/
        transcript.vtt
```

### Source Tags

Use consistent, descriptive source tags:
- `weekly-sync` - Recurring meeting series
- `project-alpha` - Project-specific meetings
- `archive-2025` - Historical imports

### Regular Processing

Run resolution and mention extraction after each batch import:

```bash
penf ingest meeting ~/new-meetings/ --source "weekly"
penf ingest meeting resolve
penf ingest meeting mentions
```

---

*This user guide covers the Go implementation of the meeting pipeline. For API details, see the API Reference.*
