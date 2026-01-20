# Penfold Ingest Pipeline

This document describes the content ingestion pipeline in Penfold, including the various pre-processing steps, entity resolution, and enrichment processes.

## Overview

The ingest pipeline transforms raw content (emails, meeting transcripts, documents) into searchable, correlated knowledge. The pipeline follows these stages:

```
Raw Content → Parsing → Entity Resolution → Enrichment → Storage → Indexing
```

## Content Types

### Meeting Transcripts

**Supported Formats:**
- WebVTT (.vtt) - Webex, Zoom transcripts
- Plain text (.txt) - Text transcripts with speaker labels
- Chat logs - Meeting chat messages

**CLI Command:**
```bash
penf ingest meeting <path> --source <tag> --platform webex|teams|zoom|google_meet
```

**Example:**
```bash
penf ingest meeting ~/meetings/TER_2025-01-15/ --source "ter-meetings"
```

### Email (EML)

**Supported Formats:**
- EML files (.eml) - Standard email format
- MBOX archives (.mbox)

**CLI Command:**
```bash
penf ingest email <path> --source <tag>
```

## Pipeline Stages

### 1. Parsing

Each content type has specialized parsers that extract:

#### Meeting Transcripts
- **VTT Parser** (`pkg/ingest/meeting/vtt_parser.go`)
  - Extracts timestamped utterances
  - Identifies speakers from voice tags
  - Calculates meeting duration

- **TXT Parser** (`pkg/ingest/meeting/txt_parser.go`)
  - Parses "Speaker: text" format
  - Normalizes speaker names (strips pronouns like "she/her")

- **Chat Parser** (`pkg/ingest/meeting/chat_parser.go`)
  - Extracts chat messages with timestamps
  - Identifies message senders

#### Email
- **EML Parser** (`pkg/ingest/eml/parser.go`)
  - Extracts headers (From, To, CC, Subject, Date)
  - Parses MIME body (text and HTML)
  - Extracts attachments

### 2. Entity Resolution

After parsing, the pipeline resolves entities to known records:

#### Participant Resolution
Matches transcript speakers and email participants to people in the database.

**Resolution Methods:**
1. **Exact Match** - Canonical name matches exactly
2. **Alias Match** - Matches known aliases
3. **Normalized Match** - Strips common suffixes (pronouns, titles)

**CLI Command:**
```bash
penf ingest meeting resolve --source <tag>
```

**Code:** `pkg/ingest/meeting/resolver.go`

#### Mention Extraction
Identifies when people are *mentioned* in content (distinct from being an attendee).

**CLI Command:**
```bash
penf ingest meeting mentions
```

**Code:** `pkg/ingest/meeting/mentions.go`

### 3. Enrichment

#### Acronym Detection

The pipeline automatically detects potential acronyms/abbreviations in transcripts that aren't in the glossary.

**Detection Rules:**
- 2-10 character uppercase sequences
- Mixed case patterns (e.g., "DBaaS", "PostgreSQL")
- Excludes common words (OK, AM, PM, etc.)
- Excludes standard technical terms (API, URL, HTTP, etc.)

**Confidence Scoring:**
- Base: 0.5
- +0.2 if appears 3+ times
- +0.1 if appears 2+ times
- +0.1 if 4+ characters

**Output:** Unknown acronyms are queued for human review.

**Code:** `pkg/ingest/meeting/acronyms.go`

#### Review Queue

Detected acronyms and other ambiguities are queued for human review:

**Question Types:**
| Type | Description | Priority |
|------|-------------|----------|
| `acronym` | Unknown term needs definition | Based on confidence |
| `person` | Ambiguous person reference | High |
| `entity` | Entity needs confirmation | Medium |
| `duplicate` | Potential duplicate content | Medium |

**CLI Commands:**
```bash
penf review questions list              # Show pending questions
penf review questions next              # Get next question
penf review questions resolve <id> <answer>  # Answer a question
penf review questions dismiss <id>      # Dismiss as not needed
penf review questions stats             # Queue statistics
```

**Code:** `pkg/reviewqueue/`

#### Glossary Integration

When an acronym question is resolved, the answer is automatically added to the glossary for query expansion during search.

**CLI Commands:**
```bash
penf glossary add <term> <expansion>    # Add term manually
penf glossary list                       # List all terms
penf glossary expand <query>            # Preview query expansion
```

**Code:** `pkg/glossary/`

### 4. Storage

Content is stored in PostgreSQL with the following structure:

#### Meetings Table
```sql
meetings (
    id, tenant_id, title, normalized_title,
    meeting_date, platform, duration_seconds,
    participant_count, participants,
    has_transcript, has_chat, has_video, has_audio,
    processing_status
)
```

#### Sources Table
```sql
sources (
    id, tenant_id, meeting_id,
    source_system, external_id,
    content_type, raw_content, content_hash,
    processing_status
)
```

#### Supporting Tables
- `meeting_participants` - Links meetings to resolved people
- `meeting_mentions` - Tracks people mentioned in transcripts
- `glossary` - Term definitions for query expansion
- `review_queue` - AI questions for human review

### 5. Indexing

After storage, content is indexed for search:

1. **Full-text Index** - PostgreSQL tsvector for keyword search
2. **Vector Embeddings** - Generated via MLX sidecar for semantic search
3. **Normalized Titles** - Meeting titles normalized for better matching

## Data Flow Diagram

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Files     │────▶│   Parser    │────▶│   Meeting   │
│ .vtt/.txt   │     │ VTT/TXT/Chat│     │   Record    │
└─────────────┘     └─────────────┘     └─────────────┘
                                              │
                    ┌─────────────────────────┼─────────────────────────┐
                    │                         │                         │
                    ▼                         ▼                         ▼
             ┌─────────────┐          ┌─────────────┐          ┌─────────────┐
             │ Participant │          │  Acronym    │          │   Source    │
             │ Resolution  │          │ Detection   │          │   Record    │
             └─────────────┘          └─────────────┘          └─────────────┘
                    │                         │
                    ▼                         ▼
             ┌─────────────┐          ┌─────────────┐
             │  meeting_   │          │  review_    │
             │ participants│          │   queue     │
             └─────────────┘          └─────────────┘
                                             │
                                             ▼ (on resolve)
                                      ┌─────────────┐
                                      │  glossary   │
                                      └─────────────┘
```

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `DB_HOST` | PostgreSQL host | localhost |
| `DB_PORT` | PostgreSQL port | 5432 |
| `DB_NAME` | Database name | penfold |
| `DB_USER` | Database user | penfold |
| `DB_PASSWORD` | Database password | (required) |
| `DB_SSLMODE` | SSL mode | prefer |

### CLI Flags

Common flags for ingest commands:

| Flag | Description |
|------|-------------|
| `--source` | Source tag identifier (required) |
| `--tenant` | Tenant ID (default: default tenant) |
| `--dry-run` | Preview without persisting |
| `--debug` | Enable debug logging |

## Best Practices

1. **Use meaningful source tags** - Tags like "ter-meetings" or "q4-archive" help filter content later.

2. **Run participant resolution after ingest** - Entities are resolved separately to allow re-running with updated people data.

3. **Review the questions queue regularly** - Acronym definitions improve search quality through query expansion.

4. **Seed the glossary with common terms** - Pre-populate known acronyms before bulk ingest:
   ```bash
   penf glossary add TER "Technical Execution Review"
   penf glossary add DBaaS "Database as a Service"
   ```

5. **Use dry-run first** - Preview what will be imported before committing:
   ```bash
   penf ingest meeting ~/meetings/ --source "test" --dry-run
   ```

## Troubleshooting

### "No meetings found"
- Check file extensions (.vtt, .txt)
- Verify the path is correct
- For directories, ensure files follow expected naming patterns

### Participant resolution has low match rate
- Add aliases to people records
- Check for pronoun suffixes in transcript (handled automatically)
- Verify people exist in database before resolution

### Too many acronym questions queued
- Pre-populate glossary with known terms
- Common technical terms are excluded by default
- Review and dismiss irrelevant questions to prevent re-detection

## Related Documentation

- [Search Guide](./search.md) - How search uses glossary expansion
- [People Management](./people.md) - Adding and managing people records
- [Review Workflows](./review.md) - Daily review process
