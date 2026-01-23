# Meeting Pipeline API Reference

**Version**: 2.0.0
**Implementation**: Go
**Package**: `github.com/otherjamesbrown/penfold/pkg/ingest/meeting`

## Overview

The meeting pipeline provides Go packages for parsing, processing, and enriching meeting content. This reference documents the core types, functions, and CLI commands.

## Package: `pkg/ingest/meeting`

### Core Types

#### TranscriptSegment

Represents a single segment of a transcript with speaker attribution.

```go
type TranscriptSegment struct {
    Speaker   string `json:"speaker,omitempty"`
    SpeakerID string `json:"speaker_id,omitempty"`
    Text      string `json:"text"`
    StartMs   int    `json:"start_ms"`
    EndMs     int    `json:"end_ms"`
}
```

#### TranscriptResult

Result of parsing a transcript file.

```go
type TranscriptResult struct {
    Segments        []TranscriptSegment `json:"segments"`
    Speakers        []string            `json:"speakers"`
    DurationSeconds int                 `json:"duration_seconds"`
    FullText        string              `json:"full_text"`
    Format          string              `json:"format"` // "vtt", "txt"
}
```

#### ChatMessage

Represents a single chat message.

```go
type ChatMessage struct {
    Timestamp time.Time `json:"timestamp"`
    Speaker   string    `json:"speaker"`
    Message   string    `json:"message"`
    URLs      []string  `json:"urls,omitempty"`
}
```

#### ChatResult

Result of parsing a chat log file.

```go
type ChatResult struct {
    Messages  []ChatMessage `json:"messages"`
    Speakers  []string      `json:"speakers"`
    URLs      []string      `json:"urls"`      // All URLs mentioned in chat
    StartTime time.Time     `json:"start_time"`
    EndTime   time.Time     `json:"end_time"`
}
```

#### Meeting

Complete meeting representation with all components.

```go
type Meeting struct {
    Title           string            `json:"title"`
    Date            time.Time         `json:"date"`
    Platform        string            `json:"platform"`
    DurationSeconds int               `json:"duration_seconds"`
    Participants    []string          `json:"participants"`
    Files           MeetingFiles      `json:"files"`
    Transcript      *TranscriptResult `json:"transcript,omitempty"`
    Chat            *ChatResult       `json:"chat,omitempty"`
}

type MeetingFiles struct {
    TranscriptPath string   `json:"transcript_path,omitempty"`
    ChatPath       string   `json:"chat_path,omitempty"`
    VideoPath      string   `json:"video_path,omitempty"`
    AudioPath      string   `json:"audio_path,omitempty"`
    OtherPaths     []string `json:"other_paths,omitempty"`
}
```

### Parsing Functions

#### ParseVTT

Parses a WebVTT format transcript file.

```go
func ParseVTT(r io.Reader) (*TranscriptResult, error)
```

**Input Format**:
```
WEBVTT

1 "Speaker Name" (speaker_id)
00:00:05.579 --> 00:00:06.858
This is the transcript text.

2 "Another Speaker"
00:00:07.000 --> 00:00:10.500
More transcript content here.
```

**Example**:
```go
file, err := os.Open("meeting.vtt")
if err != nil {
    return err
}
defer file.Close()

result, err := meeting.ParseVTT(file)
if err != nil {
    return err
}

fmt.Printf("Duration: %d seconds\n", result.DurationSeconds)
fmt.Printf("Speakers: %v\n", result.Speakers)
fmt.Printf("Segments: %d\n", len(result.Segments))
```

#### ParseTXTTranscript

Parses a plain text transcript file.

```go
func ParseTXTTranscript(r io.Reader) (*TranscriptResult, error)
```

**Input Format**:
```
0:11 : Speaker Name : First thing they said
0:45 : Another Speaker (she/her) : Response text
12:30 : Speaker Name : Later in the meeting
```

**Example**:
```go
file, _ := os.Open("Transcript_Meeting_20260123.txt")
defer file.Close()

result, err := meeting.ParseTXTTranscript(file)
```

#### ParseChatLog

Parses a chat log file with URL extraction.

```go
func ParseChatLog(r io.Reader) (*ChatResult, error)
```

**Input Format**:
```
2026-01-23 09:07 : John Smith : Hello everyone
2026-01-23 09:08 : Jane Doe : Check out <a href="https://example.com">this link</a>
-----> 2026-01-23 09:10 : Bot : Automated message
```

**Example**:
```go
file, _ := os.Open("Chat messages_Meeting_20260123.txt")
defer file.Close()

result, err := meeting.ParseChatLog(file)
if err != nil {
    return err
}

fmt.Printf("Messages: %d\n", len(result.Messages))
fmt.Printf("URLs found: %v\n", result.URLs)
fmt.Printf("Time range: %v - %v\n", result.StartTime, result.EndTime)
```

### File Scanning

#### ScanMeetingFiles

Scans a path (file or directory) and returns discovered meetings.

```go
func ScanMeetingFiles(path string) ([]*Meeting, error)
```

**Behavior**:
- Single file: Returns meeting with that file
- Directory with meeting files: Groups related files into one meeting
- Directory with subdirectories: Recursively scans each as potential meeting

**Example**:
```go
meetings, err := meeting.ScanMeetingFiles("/path/to/meetings")
if err != nil {
    return err
}

for _, m := range meetings {
    fmt.Printf("Meeting: %s (%s)\n", m.Title, m.Date.Format("2006-01-02"))
    if m.Files.TranscriptPath != "" {
        fmt.Printf("  Transcript: %s\n", m.Files.TranscriptPath)
    }
    if m.Files.ChatPath != "" {
        fmt.Printf("  Chat: %s\n", m.Files.ChatPath)
    }
}
```

#### DetectFileType

Determines the type of a meeting-related file.

```go
func DetectFileType(filename string) string
```

**Returns**: `"vtt"`, `"transcript"`, `"chat"`, `"video"`, `"audio"`, or `"unknown"`

#### ExtractMeetingInfo

Extracts meeting title and date from filename or directory name.

```go
func ExtractMeetingInfo(name string) MeetingInfo

type MeetingInfo struct {
    Title string
    Date  time.Time
}
```

**Supported patterns**:
- `Meeting Title-YYYYMMDD HHMM-1.vtt`
- `Transcript_Owner_s meeting_YYYYMMDD.txt`
- `Meeting Name - MMDDYYYY/` (directory)

### Participant Resolution

#### ParticipantResolver

Matches participant display names to known people.

```go
type ParticipantResolver struct {
    // Internal indexes
}

func NewParticipantResolver(people []Person) *ParticipantResolver
```

#### Person

Represents a person from the database.

```go
type Person struct {
    ID            int64
    CanonicalName string
    Aliases       []string
}
```

#### Match

Attempts to match a participant name to a person.

```go
func (r *ParticipantResolver) Match(participantName string) *PersonMatch

type PersonMatch struct {
    PersonID      int64
    CanonicalName string
    MatchType     MatchType  // "exact", "alias", "fuzzy"
    Confidence    float64
}
```

#### ResolveAll

Resolves a list of participant names.

```go
func (r *ParticipantResolver) ResolveAll(participants []string) ParticipantResults

type ParticipantResult struct {
    DisplayName string
    Match       *PersonMatch  // nil if unmatched
}

type ParticipantResults []ParticipantResult

func (r ParticipantResults) Stats() ResolveStats

type ResolveStats struct {
    Total     int
    Matched   int
    Unmatched int
    MatchRate float64
}
```

**Example**:
```go
people := []meeting.Person{
    {ID: 1, CanonicalName: "John Smith", Aliases: []string{"John", "JS"}},
    {ID: 2, CanonicalName: "Jane Doe", Aliases: []string{"Jane"}},
}

resolver := meeting.NewParticipantResolver(people)

// Match single name
match := resolver.Match("John Smith (he/him)")
// match.PersonID = 1, match.MatchType = "exact"

// Resolve all speakers
results := resolver.ResolveAll(transcript.Speakers)
stats := results.Stats()
fmt.Printf("Matched %d/%d (%.1f%%)\n", stats.Matched, stats.Total, stats.MatchRate*100)
```

#### NormalizeName

Cleans up a participant name by stripping pronouns.

```go
func NormalizeName(name string) string
```

Strips patterns like `(she/her)`, `(he/him)`, `(they/them)`.

### Mention Extraction

#### MentionExtractor

Extracts mentions of known people from text.

```go
type MentionExtractor struct {
    // Internal patterns
}

func NewMentionExtractor(people []Person) *MentionExtractor
```

#### Mention

Represents a person mentioned in text.

```go
type Mention struct {
    PersonID      int64
    CanonicalName string
    MatchedText   string           // Actual text that matched
    MatchType     MentionMatchType // "canonical" or "alias"
    Count         int              // Number of occurrences
    Context       string           // Surrounding text snippet
}
```

#### Extract / ExtractExcluding

Finds all mentions of known people in text.

```go
func (e *MentionExtractor) Extract(text string) []Mention

func (e *MentionExtractor) ExtractExcluding(text string, excludeIDs map[int64]bool) []Mention
```

**Example**:
```go
extractor := meeting.NewMentionExtractor(people)

// Extract all mentions
mentions := extractor.Extract(transcript.FullText)

// Extract mentions, excluding attendees
attendeeIDs := map[int64]bool{1: true, 2: true}
mentions := extractor.ExtractExcluding(transcript.FullText, attendeeIDs)

for _, m := range mentions {
    fmt.Printf("Mentioned %s (%d times): %s\n",
        m.CanonicalName, m.Count, m.Context)
}
```

### Acronym Detection

#### AcronymDetector

Detects potential acronyms in text.

```go
type AcronymDetector struct {
    KnownTerms  map[string]bool
    CommonWords map[string]bool
    MinLength   int  // default: 2
    MaxLength   int  // default: 10
}

func NewAcronymDetector() *AcronymDetector
```

#### DetectedAcronym

Represents a potential acronym found in text.

```go
type DetectedAcronym struct {
    Term    string // The acronym (uppercase)
    Context string // Surrounding text for context
    Count   int    // Number of occurrences
}
```

#### SetKnownTerms / AddKnownTerm

Configure terms to skip (from glossary).

```go
func (d *AcronymDetector) SetKnownTerms(terms []string)
func (d *AcronymDetector) AddKnownTerm(term string)
```

#### Detect / DetectInTranscript

Finds potential acronyms in text.

```go
func (d *AcronymDetector) Detect(text string) []DetectedAcronym

func (d *AcronymDetector) DetectInTranscript(
    transcript *TranscriptResult,
    minOccurrences int,
) []DetectedAcronym
```

**Example**:
```go
detector := meeting.NewAcronymDetector()

// Load known terms from glossary
detector.SetKnownTerms([]string{"API", "SQL", "MVP"})

// Detect acronyms in transcript (min 2 occurrences)
acronyms := detector.DetectInTranscript(transcript, 2)

for _, acr := range acronyms {
    fmt.Printf("Found %s (%d times): %s\n",
        acr.Term, acr.Count, acr.Context)
}
```

## CLI Reference

### penf ingest meeting

Ingest meeting transcripts into Penfold.

```bash
penf ingest meeting <path> --source <tag> [flags]
```

**Arguments**:
- `<path>`: File or directory path

**Flags**:
- `--source, -s`: Source tag identifier (required)
- `--platform`: Meeting platform: webex, teams, zoom, google_meet (default: webex)
- `--tenant-id, -t`: Tenant ID (optional)
- `--dry-run`: Preview import without persisting

**Examples**:
```bash
# Single file
penf ingest meeting ./meeting.vtt --source "project-x"

# Directory with multiple meetings
penf ingest meeting ./meetings/ --source "archive-2025"

# Dry run to preview
penf ingest meeting ./meetings/ --source "test" --dry-run
```

### penf ingest meeting resolve

Resolve meeting participants to known people.

```bash
penf ingest meeting resolve [flags]
```

**Flags**:
- `--source, -s`: Filter by source tag (optional)
- `--tenant-id, -t`: Tenant ID (optional)

**Output**:
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

### penf ingest meeting mentions

Extract mentions of people from meeting transcripts.

```bash
penf ingest meeting mentions [flags]
```

**Flags**:
- `--tenant-id, -t`: Tenant ID (optional)

**Output**:
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

## Database Schema

### meetings table

```sql
CREATE TABLE meetings (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL,
    title TEXT NOT NULL,
    normalized_title TEXT,
    meeting_date TIMESTAMPTZ,
    platform TEXT,
    duration_seconds INTEGER,
    participant_count INTEGER,
    participants JSONB DEFAULT '[]',
    source_tag TEXT,
    source_path TEXT,
    has_transcript BOOLEAN DEFAULT FALSE,
    has_chat BOOLEAN DEFAULT FALSE,
    has_video BOOLEAN DEFAULT FALSE,
    has_audio BOOLEAN DEFAULT FALSE,
    processing_status TEXT DEFAULT 'pending',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
```

### meeting_participants table

```sql
CREATE TABLE meeting_participants (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL,
    meeting_id BIGINT NOT NULL REFERENCES meetings(id),
    person_id BIGINT REFERENCES people(id),
    display_name TEXT NOT NULL,
    match_type TEXT,  -- 'exact', 'alias', 'fuzzy'
    confidence FLOAT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(meeting_id, display_name)
);
```

### meeting_mentions table

```sql
CREATE TABLE meeting_mentions (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL,
    meeting_id BIGINT NOT NULL REFERENCES meetings(id),
    source_id BIGINT NOT NULL REFERENCES sources(id),
    person_id BIGINT NOT NULL REFERENCES people(id),
    matched_text TEXT,
    match_type TEXT,  -- 'canonical', 'alias'
    context TEXT,
    mention_count INTEGER DEFAULT 1,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(meeting_id, person_id)
);
```

## Error Handling

All parsing functions return errors that can be checked:

```go
result, err := meeting.ParseVTT(file)
if err != nil {
    // Handle parsing error
    log.Printf("Failed to parse VTT: %v", err)
    return err
}
```

Common error scenarios:
- Invalid file format
- Malformed timestamps
- Empty content
- I/O errors

---

*This API reference documents the Go implementation of the meeting pipeline. For usage examples and workflows, see the User Guide.*
