# Feature Specification: Meeting Series Support

**Feature Branch**: `019-meeting-series`
**Created**: 2026-02-01
**Status**: Draft
**Input**: Meeting Series support for recurring meetings with data model and CLI commands

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Ingest Meeting with Series Assignment (Priority: P1)

A user ingests a meeting transcript and assigns it to a recurring meeting series (e.g., "TER Weekly"). If the series doesn't exist, it's automatically created. This enables organizing related meetings for future queries.

**Why this priority**: This is the core workflow - users need to associate meetings with series during ingestion. Without this, the feature has no entry point.

**Independent Test**: Can be fully tested by running `penf ingest meeting ./transcript.txt --series "TER Weekly"` and verifying the meeting is linked to the series (creating the series if needed).

**Acceptance Scenarios**:

1. **Given** no series named "TER Weekly" exists, **When** user runs `penf ingest meeting ./transcript.txt --platform teams --series "TER Weekly"`, **Then** the series is created and the meeting is linked to it
2. **Given** a series named "TER Weekly" already exists, **When** user runs `penf ingest meeting ./transcript.txt --series "TER Weekly"`, **Then** the meeting is linked to the existing series
3. **Given** a meeting transcript, **When** user runs `penf ingest meeting ./transcript.txt --title "Q4 Review" --date 2026-01-15`, **Then** the meeting is created with the specified title and date overriding any detected values

---

### User Story 2 - List and Filter Meetings by Series (Priority: P2)

A user wants to find all meetings from a specific recurring meeting (e.g., "TikTok MTC PMO") to search for past decisions or context.

**Why this priority**: This is the primary query use case - "In the TER meetings, when did we decide X?" depends on filtering by series.

**Independent Test**: Can be fully tested by running `penf meeting list --series "TER Weekly"` and verifying only meetings in that series are returned.

**Acceptance Scenarios**:

1. **Given** multiple meetings exist across different series, **When** user runs `penf meeting list --series "TER Weekly"`, **Then** only meetings belonging to "TER Weekly" are displayed
2. **Given** a series with meetings, **When** user runs `penf meeting series show <series-id>`, **Then** the series details and all its meetings are displayed

---

### User Story 3 - Manage Meeting Series (Priority: P2)

A user wants to create, list, and manage meeting series independently of ingesting meetings.

**Why this priority**: Administrative capability for managing series directly, useful for pre-creating series or cleaning up.

**Independent Test**: Can be fully tested by running `penf meeting series create "New Series"` then `penf meeting series list` to verify it appears.

**Acceptance Scenarios**:

1. **Given** no series exist, **When** user runs `penf meeting series create "TER Weekly"`, **Then** a new series is created with that name
2. **Given** series exist, **When** user runs `penf meeting series list`, **Then** all series are displayed with their names and IDs
3. **Given** a series exists with meetings, **When** user runs `penf meeting series delete <series-id>`, **Then** the series is deleted but its meetings remain (orphaned, series_id set to null)

---

### User Story 4 - Link/Unlink Existing Meetings to Series (Priority: P3)

A user has standalone meetings that were ingested without a series assignment and wants to organize them into a series after the fact.

**Why this priority**: Correction/organization workflow - less common than ingestion-time assignment but needed for data cleanup.

**Independent Test**: Can be fully tested by running `penf meeting set-series <meeting-id> "TER Weekly"` on a standalone meeting and verifying the link.

**Acceptance Scenarios**:

1. **Given** a standalone meeting exists (no series), **When** user runs `penf meeting set-series <meeting-id> "TER Weekly"`, **Then** the meeting is linked to that series (creating it if needed)
2. **Given** a meeting is linked to a series, **When** user runs `penf meeting unset-series <meeting-id>`, **Then** the meeting becomes standalone (series_id set to null)

---

### Edge Cases

- What happens when a user tries to create a series with a name that already exists? **Behavior**: Return an error indicating the series already exists.
- What happens when deleting the last series a meeting belongs to? **Behavior**: The meeting remains with series_id set to null (orphaned).
- What happens when filtering by a series name that doesn't exist? **Behavior**: Return an empty list with no error.
- What happens when `--series` flag is used with an empty string? **Behavior**: Return a validation error.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST support a MeetingSeries entity with id, name, description (optional), project_id (optional), and timestamps
- **FR-002**: System MUST support a series_id foreign key on meetings that links to MeetingSeries
- **FR-003**: System MUST allow series_id on meetings to be null (standalone meetings)
- **FR-004**: System MUST auto-create a series when `--series "Name"` is used during ingest and the series doesn't exist
- **FR-005**: System MUST match series by exact name (case-sensitive)
- **FR-006**: System MUST provide `penf meeting series list` command to display all series
- **FR-007**: System MUST provide `penf meeting series create <name>` command to create a new series
- **FR-008**: System MUST provide `penf meeting series show <id>` command to display series details and its meetings
- **FR-009**: System MUST provide `penf meeting series delete <id>` command to remove a series
- **FR-010**: System MUST orphan meetings (set series_id to null) when their series is deleted, not delete the meetings
- **FR-011**: System MUST provide `--series <name>` flag on `penf ingest meeting` command
- **FR-012**: System MUST provide `--title <title>` flag on `penf ingest meeting` to override detected title
- **FR-013**: System MUST provide `--date <date>` flag on `penf ingest meeting` to override detected date
- **FR-014**: System MUST provide `penf meeting list --series <name>` filter to show only meetings in a series
- **FR-015**: System MUST provide `penf meeting set-series <meeting-id> <series-name>` command
- **FR-016**: System MUST provide `penf meeting unset-series <meeting-id>` command
- **FR-017**: System MUST reject series creation when a series with the same name already exists

### Key Entities

- **MeetingSeries**: Represents a recurring meeting series (e.g., "TER Weekly", "TikTok MTC PMO"). Contains: id (unique identifier with `ms-` prefix), name (display name, unique), description (optional context), project_id (optional link to project), created_at, updated_at.
- **Meeting** (existing, extended): Content item representing a single meeting instance. Extended with: series_id (optional FK to MeetingSeries).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Users can ingest a meeting and assign it to a series in a single command
- **SC-002**: Users can filter meetings by series name and see only relevant results
- **SC-003**: Users can organize standalone meetings into series after initial ingestion
- **SC-004**: All series management operations (create, list, show, delete) complete successfully
- **SC-005**: Deleting a series preserves all associated meetings as standalone content

## Assumptions

- Series names are unique across the system (no duplicate series names allowed)
- Series matching is case-sensitive (exact match required)
- The `ms-` prefix convention follows existing entity ID patterns in the codebase
- The existing meeting ingest pipeline can be extended without breaking current functionality
- The `--title` and `--date` flags override AI-detected values from transcript parsing
