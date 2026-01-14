# Meeting Pipeline API Reference

**Version**: 1.0.0
**Base URL**: `/api/v1/meetings`
**Authentication**: JWT Bearer Token
**Content-Type**: `application/json`

## Overview

The Meeting Pipeline API provides programmatic access to upload meetings, monitor processing, search content, and manage review workflows. All endpoints support async operations with real-time status updates.

## Authentication

All API requests require authentication using JWT Bearer tokens:

```bash
curl -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  https://api.example.com/api/v1/meetings/
```

## Core Endpoints

### Meeting Management

#### Upload Meeting
```http
POST /api/v1/meetings/upload
```

Upload audio/video files or documents for processing.

**Request Body** (multipart/form-data):
```json
{
  "file": "<binary file data>",
  "metadata": {
    "title": "Q4 Planning Meeting",
    "meeting_date": "2026-01-14T14:00:00Z",
    "participants": ["john@example.com", "jane@example.com"],
    "project_context": "Q4 Planning",
    "privacy_level": "internal",
    "description": "Quarterly planning discussion"
  }
}
```

**Response**:
```json
{
  "meeting_id": "550e8400-e29b-41d4-a716-446655440000",
  "upload_status": "completed",
  "processing_status": "queued",
  "estimated_completion_time": "2026-01-14T14:30:00Z",
  "tracking_url": "/api/v1/meetings/550e8400-e29b-41d4-a716-446655440000/status"
}
```

#### Get Meeting Status
```http
GET /api/v1/meetings/{meeting_id}/status
```

Monitor processing progress and current status.

**Response**:
```json
{
  "meeting_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "processing",
  "current_phase": "transcription",
  "progress": 65,
  "estimated_completion": "2026-01-14T14:25:00Z",
  "phases": {
    "file_validation": {"status": "completed", "confidence": 100},
    "transcription": {"status": "in_progress", "confidence": null},
    "speaker_identification": {"status": "pending", "confidence": null},
    "content_analysis": {"status": "pending", "confidence": null},
    "search_indexing": {"status": "pending", "confidence": null}
  },
  "quality_indicators": {
    "audio_quality": 85,
    "speech_clarity": 78,
    "background_noise": 15
  }
}
```

#### Get Meeting Details
```http
GET /api/v1/meetings/{meeting_id}
```

Retrieve complete meeting information after processing.

**Response**:
```json
{
  "meeting_id": "550e8400-e29b-41d4-a716-446655440000",
  "metadata": {
    "title": "Q4 Planning Meeting",
    "meeting_date": "2026-01-14T14:00:00Z",
    "duration_seconds": 3600,
    "file_size_bytes": 156934144,
    "privacy_level": "internal"
  },
  "transcript": {
    "text": "Complete transcript text...",
    "confidence_score": 92,
    "word_count": 2847,
    "speaker_segments": [
      {
        "speaker": "John Smith",
        "start_time": 0,
        "end_time": 45.3,
        "text": "Let's start with our Q4 objectives...",
        "confidence": 95
      }
    ]
  },
  "summary": {
    "overview": "Meeting covered Q4 objectives, budget allocation, and team assignments...",
    "key_points": [
      "Q4 revenue target set at $2.5M",
      "Marketing budget increased by 15%",
      "Three new hires approved for engineering team"
    ],
    "action_items": [
      {
        "item": "Finalize marketing budget proposal",
        "assignee": "jane@example.com",
        "due_date": "2026-01-21",
        "priority": "high"
      }
    ],
    "decisions": [
      {
        "decision": "Approved 15% marketing budget increase",
        "context": "Based on Q3 performance metrics",
        "timestamp": 1547.2
      }
    ]
  },
  "participants": [
    {
      "person_id": "person-123",
      "name": "John Smith",
      "email": "john@example.com",
      "speaking_time": 1240.5,
      "contribution_percentage": 34,
      "identification_confidence": 96
    }
  ],
  "topics": [
    {
      "topic": "Q4 Budget Planning",
      "relevance_score": 95,
      "time_segments": [[120, 890], [1200, 1450]],
      "project_context": "Q4 Planning Initiative"
    }
  ],
  "quality_metrics": {
    "overall_confidence": 92,
    "transcription_confidence": 94,
    "speaker_id_confidence": 89,
    "content_analysis_confidence": 91
  }
}
```

#### List Meetings
```http
GET /api/v1/meetings?page=1&page_size=20&status=completed&project=Q4%20Planning
```

**Query Parameters**:
- `page` (integer): Page number (default: 1)
- `page_size` (integer): Items per page (default: 20, max: 100)
- `status` (string): Filter by processing status
- `project` (string): Filter by project context
- `participant` (string): Filter by participant email
- `date_from` (ISO date): Filter meetings after date
- `date_to` (ISO date): Filter meetings before date
- `privacy_level` (string): Filter by privacy level
- `min_confidence` (integer): Minimum quality confidence (0-100)

**Response**:
```json
{
  "meetings": [
    {
      "meeting_id": "550e8400-e29b-41d4-a716-446655440000",
      "title": "Q4 Planning Meeting",
      "meeting_date": "2026-01-14T14:00:00Z",
      "status": "completed",
      "participants_count": 5,
      "duration_seconds": 3600,
      "confidence_score": 92,
      "has_action_items": true,
      "created_at": "2026-01-14T15:30:00Z"
    }
  ],
  "pagination": {
    "page": 1,
    "page_size": 20,
    "total_items": 156,
    "total_pages": 8,
    "has_next": true,
    "has_previous": false
  }
}
```

### Search and Discovery

#### Search Meetings
```http
POST /api/v1/meetings/search
```

Semantic and keyword search across meeting content.

**Request Body**:
```json
{
  "query": "budget allocation marketing",
  "filters": {
    "date_range": {
      "start": "2026-01-01T00:00:00Z",
      "end": "2026-01-31T23:59:59Z"
    },
    "participants": ["john@example.com"],
    "projects": ["Q4 Planning"],
    "content_types": ["decisions", "action_items"],
    "min_confidence": 80,
    "privacy_levels": ["internal", "public"]
  },
  "search_type": "semantic",
  "max_results": 50
}
```

**Response**:
```json
{
  "results": [
    {
      "meeting_id": "550e8400-e29b-41d4-a716-446655440000",
      "relevance_score": 94,
      "match_type": "semantic",
      "highlights": [
        {
          "type": "transcript",
          "text": "We need to allocate the marketing budget more effectively...",
          "timestamp": 1234.5,
          "speaker": "John Smith",
          "confidence": 92
        },
        {
          "type": "action_item",
          "text": "Finalize marketing budget proposal",
          "assignee": "jane@example.com",
          "confidence": 95
        }
      ],
      "meeting_summary": {
        "title": "Q4 Planning Meeting",
        "date": "2026-01-14T14:00:00Z",
        "participants_count": 5,
        "relevance_reason": "Direct discussion of marketing budget allocation"
      }
    }
  ],
  "search_metadata": {
    "query": "budget allocation marketing",
    "total_results": 23,
    "search_time_ms": 145,
    "semantic_expansion": ["budget", "funding", "allocation", "distribution", "marketing", "advertising"]
  }
}
```

### Manual Review and Corrections

#### Get Review Queue
```http
GET /api/v1/review/review-queue?page=1&page_size=20&review_type=transcription_quality&assigned_to=me
```

**Response**:
```json
{
  "review_items": [
    {
      "review_id": "review-123",
      "meeting_id": "550e8400-e29b-41d4-a716-446655440000",
      "review_type": "speaker_identification",
      "priority": "high",
      "confidence_issues": [
        {
          "issue_type": "low_speaker_confidence",
          "timestamp": 234.5,
          "current_speaker": "Unknown Speaker 2",
          "suggested_speakers": ["Jane Doe", "Bob Wilson"],
          "confidence_scores": [0.65, 0.58]
        }
      ],
      "estimated_effort": "5 minutes",
      "created_at": "2026-01-14T15:35:00Z",
      "assigned_to": "reviewer@example.com"
    }
  ],
  "statistics": {
    "total_pending": 12,
    "total_in_review": 3,
    "total_overdue": 1,
    "avg_resolution_time": "8 minutes"
  }
}
```

#### Submit Transcript Edits
```http
POST /api/v1/review/transcript/{meeting_id}/edit
```

**Request Body**:
```json
{
  "edits": [
    {
      "segment_id": "segment-456",
      "new_text": "We need to finalize the marketing budget by Friday",
      "new_speaker": "John Smith",
      "start_time": 1234.5,
      "end_time": 1239.8,
      "confidence_override": 95
    }
  ],
  "change_summary": "Corrected misheard words and speaker attribution",
  "reviewer_notes": "Audio quality was poor at this timestamp"
}
```

**Response**:
```json
{
  "version_id": "version-789",
  "changes_applied": 1,
  "new_version_number": 3,
  "confidence_improvement": 4.2,
  "quality_score": 96
}
```

#### Submit User Feedback
```http
POST /api/v1/review/feedback/submit
```

**Request Body**:
```json
{
  "meeting_id": "550e8400-e29b-41d4-a716-446655440000",
  "feedback_type": "transcription_quality",
  "component": "speaker_identification",
  "rating": 4,
  "confidence_rating": 3,
  "accuracy_rating": 4,
  "comments": "Speaker identification was mostly accurate but confused two similar voices",
  "specific_issues": [
    {
      "timestamp": 1234.5,
      "issue_type": "wrong_speaker",
      "expected": "Jane Doe",
      "actual": "John Smith"
    }
  ],
  "suggestions": "Consider voice training for frequent participants"
}
```

## Version Control

#### Get Version History
```http
GET /api/v1/meetings/{meeting_id}/versions
```

**Response**:
```json
{
  "versions": [
    {
      "version_id": "version-789",
      "version_number": 3,
      "change_type": "manual_edit",
      "changed_by": "reviewer@example.com",
      "created_at": "2026-01-14T16:15:00Z",
      "change_summary": "Corrected speaker identification in segments 45-67",
      "diff_summary": "5 speaker changes, 12 text corrections"
    }
  ]
}
```

#### Compare Versions
```http
GET /api/v1/meetings/versions/{version1_id}/compare/{version2_id}
```

#### Rollback to Version
```http
POST /api/v1/meetings/{meeting_id}/rollback/{version_id}
```

## Analytics and Insights

#### Get Processing Analytics
```http
GET /api/v1/analytics/processing?period=30d
```

**Response**:
```json
{
  "processing_metrics": {
    "total_meetings_processed": 156,
    "average_processing_time_minutes": 28.5,
    "success_rate": 0.94,
    "average_confidence_score": 87.3,
    "total_processing_hours": 74.1
  },
  "quality_trends": {
    "transcription_confidence": [85, 87, 89, 88, 91],
    "speaker_identification_accuracy": [78, 81, 85, 87, 89],
    "content_analysis_confidence": [82, 84, 87, 88, 90]
  },
  "usage_patterns": {
    "peak_upload_hours": [9, 10, 14, 15],
    "average_meeting_duration": 52.3,
    "most_common_file_types": ["mp4", "mp3", "mov"]
  }
}
```

## WebSocket Real-Time Updates

### Connect to Processing Updates
```javascript
const ws = new WebSocket('wss://api.example.com/ws/meetings/550e8400-e29b-41d4-a716-446655440000');

ws.onmessage = function(event) {
  const update = JSON.parse(event.data);
  if (update.type === 'processing_progress') {
    console.log(`Progress: ${update.progress}% - ${update.current_phase}`);
  }
};
```

**Message Types**:
- `processing_progress`: Processing phase updates
- `quality_alert`: Low confidence warnings
- `completion_notification`: Processing finished
- `error_notification`: Processing errors

## Error Handling

### Standard Error Response
```json
{
  "error": {
    "code": "PROCESSING_FAILED",
    "message": "Transcription failed due to poor audio quality",
    "details": {
      "phase": "transcription",
      "error_type": "audio_quality_insufficient",
      "suggested_actions": ["improve audio quality", "try manual upload", "contact support"]
    },
    "timestamp": "2026-01-14T15:45:00Z",
    "request_id": "req-12345"
  }
}
```

### Error Codes

| Code | Description | Action |
|------|-------------|--------|
| `INVALID_FILE_FORMAT` | Unsupported file type | Use MP4, MP3, WAV, MOV, PDF, DOCX |
| `FILE_SIZE_EXCEEDED` | File larger than 2GB | Split file or compress |
| `PROCESSING_FAILED` | Processing error | Check audio quality, retry, or contact support |
| `INSUFFICIENT_PERMISSIONS` | Access denied | Check privacy level and user permissions |
| `QUOTA_EXCEEDED` | Processing quota reached | Wait for quota reset or upgrade plan |
| `INVALID_CONFIDENCE_THRESHOLD` | Invalid confidence parameter | Use value between 0-100 |

## Rate Limits

- **Upload**: 10 files per hour per user
- **Search**: 100 requests per minute per user
- **Status checks**: 60 requests per minute per meeting
- **Analytics**: 20 requests per hour per user

Rate limit headers:
```http
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 87
X-RateLimit-Reset: 1640995200
```

## SDK and Client Libraries

### Python SDK
```python
from penfold_meeting_pipeline import MeetingClient

client = MeetingClient(
    api_url="https://api.example.com",
    api_token="your_jwt_token"
)

# Upload meeting
meeting = await client.upload_meeting(
    file_path="meeting.mp4",
    title="Q4 Planning Meeting",
    participants=["john@example.com", "jane@example.com"]
)

# Monitor progress
status = await client.get_status(meeting.id)
print(f"Progress: {status.progress}%")

# Search meetings
results = await client.search(
    query="budget allocation",
    min_confidence=80
)
```

### JavaScript SDK
```javascript
import { MeetingClient } from '@penfold/meeting-pipeline-js';

const client = new MeetingClient({
  apiUrl: 'https://api.example.com',
  apiToken: 'your_jwt_token'
});

// Upload with progress tracking
const meeting = await client.uploadMeeting({
  file: fileInput.files[0],
  title: 'Q4 Planning Meeting',
  onProgress: (progress) => console.log(`${progress}%`)
});

// Real-time updates
client.subscribe(meeting.id, (update) => {
  if (update.type === 'processing_complete') {
    console.log('Meeting processing finished!');
  }
});
```

---

*This API reference provides complete coverage of the Meeting Pipeline REST API. For implementation examples and integration patterns, see the Integration Guide.*