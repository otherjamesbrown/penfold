# Quickstart: Meeting Series Support

**Feature**: 019-meeting-series
**Date**: 2026-02-01

## Overview

This feature adds the ability to group meetings into recurring series (e.g., "TER Weekly", "TikTok MTC PMO") for better organization and querying.

## Usage Examples

### 1. Ingest a Meeting with Series Assignment

```bash
# Ingest meeting and assign to series (creates series if doesn't exist)
penf ingest meeting ./transcripts/ter-2026-01-15/ \
  --source downloads \
  --platform teams \
  --series "TER Weekly"

# Override detected title and date
penf ingest meeting ./transcripts/adhoc-meeting.vtt \
  --source downloads \
  --platform zoom \
  --title "Q4 Planning Session" \
  --date 2026-01-20 \
  --series "Product Planning"
```

### 2. Manage Series

```bash
# List all series
penf meeting series list

# Create a series manually
penf meeting series create "TikTok MTC PMO"

# Show series details with its meetings
penf meeting series show ms-abc123

# Delete a series (meetings are preserved, just unlinked)
penf meeting series delete ms-abc123
```

### 3. Filter Meetings by Series

```bash
# List all meetings in a series
penf meeting list --series "TER Weekly"

# Combine with other filters
penf meeting list --series "TER Weekly" --after 2026-01-01
```

### 4. Link/Unlink Existing Meetings

```bash
# Add a standalone meeting to a series
penf meeting set-series mtg-xyz789 "TER Weekly"

# Remove a meeting from its series
penf meeting unset-series mtg-xyz789
```

## Key Behaviors

1. **Auto-creation**: Using `--series "Name"` during ingest creates the series if it doesn't exist
2. **Exact matching**: Series names are matched exactly (case-sensitive)
3. **Orphan preservation**: Deleting a series keeps its meetings as standalone items
4. **Unique names**: Series names must be unique across the system

## Database Changes

New `meeting_series` table:
- `id` (TEXT, PK) - e.g., `ms-abc123`
- `name` (TEXT, UNIQUE)
- `description` (TEXT, optional)
- `project_id` (UUID, optional FK)
- `created_at`, `updated_at` (TIMESTAMPTZ)

Extended `meetings` table:
- `series_id` (TEXT, nullable FK to meeting_series)

## API Changes

New gRPC methods in IngestService:
- `CreateSeries(CreateSeriesRequest) -> CreateSeriesResponse`
- `ListSeries(ListSeriesRequest) -> ListSeriesResponse`
- `GetSeries(GetSeriesRequest) -> GetSeriesResponse`
- `DeleteSeries(DeleteSeriesRequest) -> DeleteSeriesResponse`
- `SetMeetingSeries(SetMeetingSeriesRequest) -> SetMeetingSeriesResponse`
- `UnsetMeetingSeries(UnsetMeetingSeriesRequest) -> UnsetMeetingSeriesResponse`

Extended `IngestMeetingRequest`:
- `series_name` (string, optional)
- `title_override` (string, optional)
- `date_override` (string, optional)
