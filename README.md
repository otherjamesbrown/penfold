# Meeting Pipeline - Phase 1 Infrastructure

Core infrastructure setup for the Penfold meeting upload and processing pipeline.

## Features Implemented (Phase 1)

✅ **Core Infrastructure**
- FastAPI application with async support
- PostgreSQL database with pgvector extension
- SQLAlchemy async models for all meeting entities
- Procrastinate job queue for background processing
- Docker development environment
- Configuration management with environment variables
- Health check endpoints

## Getting Started

### Prerequisites
- Python 3.12+
- PostgreSQL 16+ with pgvector
- Docker and Docker Compose (recommended)

### Development Setup

1. **Clone and setup environment:**
```bash
git clone <repository>
cd penfold
cp .env.example .env
# Edit .env with your settings
```

2. **Using Docker (Recommended):**
```bash
docker-compose up -d postgres redis
docker-compose up api
```

3. **Local development:**
```bash
python -m venv venv
source venv/bin/activate
pip install -r requirements.txt

# Start PostgreSQL and create database
createdb penfold_dev
psql penfold_dev -c "CREATE EXTENSION vector;"

# Run the application
uvicorn app.main:app --reload
```

### Verify Installation

Check health endpoint:
```bash
curl http://localhost:8000/health
```

Expected response:
```json
{
  "status": "healthy",
  "service": "meeting-pipeline",
  "version": "1.0.0",
  "components": {
    "database": "connected",
    "job_queue": "running",
    "file_storage": "available"
  }
}
```

## Project Structure

```
app/
├── main.py              # FastAPI application
├── config.py            # Configuration management
├── database.py          # Database models and connection
├── jobs.py              # Background job definitions
├── api/                 # API routes
│   ├── upload_routes.py # File upload endpoints (placeholder)
│   ├── search_routes.py # Search endpoints (placeholder)
│   └── review_routes.py # Review endpoints (placeholder)
├── upload/              # Upload handling (Phase 2)
├── transcription/       # Audio processing (Phase 3)
├── analysis/            # AI analysis (Phase 4)
├── search/              # Search implementation (Phase 5)
├── ui/                  # User interface (Phase 6)
└── review/              # Manual review (Phase 6)
```

## Database Schema

Core entities implemented:
- `meeting_files` - Uploaded meeting recordings
- `processing_jobs` - Background job tracking
- `meeting_transcripts` - Transcribed content with embeddings
- `meeting_participants` - Speaker identification
- `meeting_summaries` - AI-generated summaries
- `meeting_topics` - Topic extraction and project linking
- `review_queue` - Manual review tasks
- `meeting_insights` - Business insights

## Next Steps

**Phase 2** - File Upload Implementation (pe-eer)
- TUS resumable upload protocol
- File validation and privacy controls
- Upload progress tracking

**Phase 3** - Audio Transcription Pipeline (pe-1le)
- WhisperX local transcription
- Speaker diarization with Pyannote
- Google Cloud Speech-to-Text fallback

See implementation beads for detailed task breakdown.

## Architecture

This implementation follows the technical decisions from `specs/005-meeting-pipeline/research.md`:
- **Database**: PostgreSQL 16+ with pgvector for semantic search
- **Job Queue**: Procrastinate with PostgreSQL backend
- **API Framework**: FastAPI with async support
- **Local-First**: Designed for Mac M4 development environment
- **Privacy Controls**: Multi-level encryption support

## Development Workflow

1. Claim next ready bead: `bd ready`
2. Update status: `bd update <bead-id> --status=in_progress`
3. Implement features according to bead description
4. Test implementation
5. Complete bead: `bd close <bead-id>`
6. Check newly available work: `bd ready`