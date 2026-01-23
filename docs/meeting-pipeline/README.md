# Meeting Pipeline Documentation

**Version**: 2.0.0
**Last Updated**: 2026-01-23
**Implementation**: Go
**Status**: Production Ready

## Overview

The Meeting Pipeline processes meeting transcripts and chat logs from video conferencing platforms (Webex, Teams, Zoom, Google Meet) into searchable, analyzable content with participant identification and entity extraction.

## Architecture

The meeting pipeline is implemented in Go with the following components:

```
pkg/ingest/meeting/
  types.go          - Core data structures
  scanner.go        - File/directory scanning and meeting detection
  vtt_parser.go     - WebVTT transcript parsing
  txt_parser.go     - Plain text transcript parsing
  chat_parser.go    - Chat log parsing with URL extraction
  resolver.go       - Participant-to-person resolution
  mentions.go       - Person mention extraction
  acronyms.go       - Unknown acronym detection

cmd/penf/cmd/
  ingest_meeting.go - CLI commands for meeting ingestion

services/worker/
  workflows/content.go - Temporal workflow for content processing
```

## Key Capabilities

- **Transcript Parsing**: WebVTT (.vtt) and plain text formats with speaker identification
- **Chat Log Processing**: Extract messages, speakers, timestamps, and URLs
- **Meeting Discovery**: Automatic grouping of related files (transcript + chat + video)
- **Participant Resolution**: Match display names to known people via aliases
- **Mention Extraction**: Find references to people in meeting content (excluding attendees)
- **Acronym Detection**: Identify unknown acronyms and queue for glossary review
- **Temporal Workflows**: Durable content processing with saga compensation

## Supported File Formats

### Transcripts
- **WebVTT (.vtt)**: Standard format from Webex/Zoom with speaker attribution and timestamps
- **Plain Text (.txt)**: Format `timestamp : Speaker Name : text`

### Chat Logs
- **Plain Text (.txt)**: Format `YYYY-MM-DD HH:MM : Speaker Name : message`
- Supports HTML links and URL extraction

### Video/Audio (metadata only)
- MP4, WebM, MOV, AVI (video)
- M4A, MP3, WAV (audio)

## Quick Start

### CLI Usage

```bash
# Ingest a single transcript
penf ingest meeting ./meeting.vtt --source "project-x"

# Ingest a meeting directory (transcript + chat)
penf ingest meeting ./MeetingFolder/ --source "weekly-sync"

# Preview without importing (dry run)
penf ingest meeting ./meetings/ --source "test" --dry-run

# Resolve participants to known people
penf ingest meeting resolve

# Extract mentions of people from transcripts
penf ingest meeting mentions
```

### Programmatic Usage (Go)

```go
import "github.com/otherjamesbrown/penfold/pkg/ingest/meeting"

// Scan for meetings in a directory
meetings, err := meeting.ScanMeetingFiles("/path/to/meetings")

// Parse a VTT transcript
file, _ := os.Open("meeting.vtt")
transcript, err := meeting.ParseVTT(file)

// Parse a chat log
chatFile, _ := os.Open("chat.txt")
chat, err := meeting.ParseChatLog(chatFile)

// Resolve participants to people
resolver := meeting.NewParticipantResolver(people)
results := resolver.ResolveAll(transcript.Speakers)

// Extract mentions (excluding attendees)
extractor := meeting.NewMentionExtractor(people)
mentions := extractor.ExtractExcluding(transcript.FullText, attendeeIDs)

// Detect unknown acronyms
detector := meeting.NewAcronymDetector()
detector.SetKnownTerms(glossaryTerms)
acronyms := detector.DetectInTranscript(transcript, minOccurrences)
```

## Processing Pipeline

1. **File Scanning** - Detect meeting files and group related content
2. **Transcript Parsing** - Extract text, speakers, and timestamps
3. **Chat Parsing** - Extract messages and URLs
4. **Database Storage** - Store meeting metadata and sources
5. **Participant Resolution** - Match speakers to known people
6. **Mention Extraction** - Find people discussed in content
7. **Acronym Detection** - Queue unknown terms for review
8. **Content Enrichment** - Generate embeddings and summaries via Temporal workflows

## Documentation Structure

### For Developers
- [**API Reference**](./api-reference.md) - Go package documentation and CLI reference
- [**User Guide**](./user-guide.md) - End-to-end usage guide

### Source Files
- `pkg/ingest/meeting/` - Core meeting processing packages
- `cmd/penf/cmd/ingest_meeting.go` - CLI implementation
- `services/worker/workflows/content.go` - Temporal content workflow

## Database Schema

Meetings are stored across several tables:

- `meetings` - Meeting metadata (title, date, platform, participants)
- `sources` - Transcript and chat content
- `meeting_participants` - Participant resolution results
- `meeting_mentions` - People mentioned in content
- `review_queue` - Acronyms pending glossary review

## System Requirements

- **Go**: 1.22+
- **PostgreSQL**: 16+ with pgvector extension
- **Temporal**: For content workflow orchestration
- **File Formats**: VTT, TXT, MP4, MP3, WAV, MOV

## Related Documentation

- [Content Ingestion Workflow](/services/worker/workflows/content.go)
- [Review Queue](/pkg/reviewqueue/)
- [Glossary](/pkg/glossary/)

---

*This documentation covers the Go implementation of the Meeting Pipeline for processing meeting transcripts into searchable, enriched content.*
