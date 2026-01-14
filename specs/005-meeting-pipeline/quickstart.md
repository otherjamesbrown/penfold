# Meeting Pipeline Quickstart Guide

**Last Updated**: 2026-01-14
**Prerequisites**: Python 3.12+, PostgreSQL 16+, Mac M4 with 32GB RAM

## Development Environment Setup

### 1. Core Dependencies

```bash
# Create virtual environment
python3.12 -m venv venv
source venv/bin/activate

# Core framework dependencies
pip install fastapi[all] uvicorn
pip install asyncpg sqlalchemy[asyncio] alembic
pip install procrastinate[asyncpg]

# AI and audio processing
pip install whisperx torch torchaudio
pip install pyannote.audio
pip install google-cloud-speech
pip install librosa soundfile pydub

# File handling and utilities
pip install tuspy aiofiles
pip install cryptography
pip install tenacity psutil
pip install python-multipart
```

### 2. Database Setup

```bash
# Install PostgreSQL with pgvector (Mac M4)
brew install postgresql@16
brew install pgvector

# Start PostgreSQL
brew services start postgresql@16

# Create development database
createdb penfold_dev

# Install pgvector extension
psql penfold_dev -c "CREATE EXTENSION vector;"
```

### 3. Environment Configuration

Create `.env` file:
```bash
# Database
DATABASE_URL=postgresql://localhost/penfold_dev

# File Storage
UPLOAD_DIR=/Users/$USER/penfold-uploads
PROCESSED_DIR=/Users/$USER/penfold-processed
MAX_UPLOAD_SIZE=2147483648  # 2GB

# Speech-to-Text
WHISPER_MODEL_SIZE=large-v3
GOOGLE_CLOUD_CREDENTIALS_PATH=/path/to/credentials.json
STT_CONFIDENCE_THRESHOLD=0.8

# Privacy & Security
ENCRYPTION_KEY_PATH=/Users/$USER/.penfold/encryption-keys
JWT_SECRET_KEY=your-jwt-secret-key-here

# Processing
MAX_CONCURRENT_JOBS=20
MAX_CPU_INTENSIVE_JOBS=4
JOB_TIMEOUT_SECONDS=7200  # 2 hours

# Event System
REDIS_URL=redis://localhost:6379/0  # Optional: for external events
```

### 4. Directory Structure

```bash
mkdir -p ~/penfold-uploads/{temp,processed,encrypted}
mkdir -p ~/.penfold/encryption-keys
mkdir -p ~/penfold-logs
```

## Quick Start Commands

### 1. Initialize Database Schema

```python
# database/schema.py
from sqlalchemy.ext.asyncio import create_async_engine, AsyncSession
from sqlalchemy.orm import sessionmaker
import asyncio

async def init_database():
    engine = create_async_engine("postgresql+asyncpg://localhost/penfold_dev")

    # Create tables
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)

    # Initialize job queue tables
    await procrastinate.schema.apply_schema(engine)

# Run initialization
asyncio.run(init_database())
```

### 2. Start Development Services

```bash
# Terminal 1: Start API server
uvicorn app.main:app --reload --port 8000

# Terminal 2: Start job worker
python -m procrastinate worker

# Terminal 3: Start development dashboard (optional)
streamlit run dashboard/main.py --server.port 8501
```

### 3. Test File Upload

```bash
# Using curl to test basic upload
curl -X POST http://localhost:8000/api/v1/meetings/upload \
  -H "Content-Type: application/json" \
  -d '{
    "filename": "test-meeting.mp3",
    "file_size": 52428800,
    "privacy_level": "team_only",
    "metadata": {
      "meeting_title": "Development Team Standup",
      "meeting_date": "2024-01-14T10:00:00Z",
      "participants": ["Alice", "Bob", "Carol"]
    }
  }'

# Response will include upload_url for TUS resumable upload
```

### 4. Monitor Processing Jobs

```python
# Check job status via API
import httpx
import asyncio

async def check_job_status(meeting_id: str):
    async with httpx.AsyncClient() as client:
        response = await client.get(f"http://localhost:8000/api/v1/meetings/{meeting_id}/status")
        return response.json()

# Or use Server-Sent Events for real-time updates
async def stream_progress(meeting_id: str):
    async with httpx.AsyncClient() as client:
        async with client.stream("GET", f"http://localhost:8000/api/v1/meetings/{meeting_id}/stream") as response:
            async for line in response.aiter_lines():
                if line.startswith("data: "):
                    print(json.loads(line[6:]))
```

## Development Workflow

### 1. Adding New Processing Steps

```python
# processors/base.py
from abc import ABC, abstractmethod
import procrastinate

app = procrastinate.App(connector=procrastinate.AiopgConnector())

class BaseProcessor(ABC):
    @abstractmethod
    async def process(self, meeting_id: str, input_data: dict) -> dict:
        pass

# processors/transcription.py
@app.task(queue="transcription", priority=10)
async def transcribe_meeting(meeting_id: str, file_path: str):
    processor = TranscriptionProcessor()
    result = await processor.process(meeting_id, {"file_path": file_path})

    # Emit completion event
    await emit_event("transcription_completed", {
        "meeting_id": meeting_id,
        "confidence": result["confidence"],
        "processing_time": result["processing_time"]
    })

    return result
```

### 2. Database Migrations

```bash
# Generate migration
alembic revision --autogenerate -m "Add meeting entities"

# Apply migration
alembic upgrade head

# For pgvector indexes
alembic revision -m "Add vector indexes" --depends-on=previous_revision_id
```

### 3. Testing Strategy

```python
# tests/test_transcription.py
import pytest
from processors.transcription import TranscriptionProcessor

@pytest.mark.asyncio
async def test_local_whisper_processing():
    processor = TranscriptionProcessor(use_local=True)

    # Test with sample audio file
    result = await processor.process("test-meeting-id", {
        "file_path": "tests/fixtures/sample-meeting.wav"
    })

    assert result["confidence"] > 0.8
    assert "speaker_segments" in result
    assert len(result["speaker_segments"]) > 0

# Run tests
pytest tests/ -v --asyncio-mode=auto
```

### 4. Local AI Model Setup

```python
# ai/whisper_local.py
import whisperx
import torch

class LocalWhisperProcessor:
    def __init__(self, model_size="large-v3", device="cpu"):
        self.device = device
        if torch.backends.mps.is_available():
            self.device = "mps"  # Mac M4 optimization

        self.model = whisperx.load_model(model_size, device=self.device)
        self.align_model, self.align_metadata = whisperx.load_align_model(
            language_code="en", device=self.device
        )
        self.diarize_model = whisperx.load_diarization_model(device=self.device)

    async def transcribe(self, audio_path: str) -> dict:
        # Load and transcribe
        audio = whisperx.load_audio(audio_path)
        result = self.model.transcribe(audio, batch_size=16)

        # Align whisper output
        result = whisperx.align(result["segments"], self.align_model,
                               self.align_metadata, audio, self.device)

        # Speaker diarization
        diarize_segments = self.diarize_model(audio)
        result = whisperx.assign_word_speakers(diarize_segments, result)

        return {
            "transcript": result,
            "confidence": self._calculate_confidence(result),
            "speaker_count": len(set(s.get("speaker", "Unknown") for s in result["segments"]))
        }
```

## Configuration Examples

### 1. Production Environment Variables

```bash
# .env.production
DATABASE_URL=postgresql://user:pass@prod-db:5432/penfold
REDIS_URL=redis://prod-redis:6379/0

# Use cloud storage for large files
STORAGE_BACKEND=s3
S3_BUCKET=penfold-meetings-prod
AWS_REGION=us-west-2

# Enhanced security
ENCRYPTION_BACKEND=vault
VAULT_URL=https://vault.company.com
JWT_ALGORITHM=RS256
```

### 2. Docker Development

```dockerfile
# Dockerfile
FROM python:3.12-slim

RUN apt-get update && apt-get install -y \
    postgresql-client \
    ffmpeg \
    && rm -rf /var/lib/apt/lists/*

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

WORKDIR /app
COPY . .

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]
```

```yaml
# docker-compose.yml
version: '3.8'
services:
  api:
    build: .
    ports:
      - "8000:8000"
    environment:
      - DATABASE_URL=postgresql://postgres:password@db:5432/penfold
    depends_on:
      - db
      - redis
    volumes:
      - ./uploads:/app/uploads

  worker:
    build: .
    command: python -m procrastinate worker
    environment:
      - DATABASE_URL=postgresql://postgres:password@db:5432/penfold
    depends_on:
      - db

  db:
    image: pgvector/pgvector:pg16
    environment:
      - POSTGRES_DB=penfold
      - POSTGRES_PASSWORD=password
    volumes:
      - postgres_data:/var/lib/postgresql/data

volumes:
  postgres_data:
```

## Monitoring and Debugging

### 1. Health Check Endpoints

```python
# app/health.py
@app.get("/health")
async def health_check():
    return {
        "status": "healthy",
        "timestamp": datetime.utcnow(),
        "version": "1.0.0",
        "components": {
            "database": await check_database(),
            "job_queue": await check_job_queue(),
            "whisper_model": await check_whisper_model()
        }
    }
```

### 2. Logging Configuration

```python
# config/logging.py
import structlog

structlog.configure(
    processors=[
        structlog.stdlib.filter_by_level,
        structlog.processors.JSONRenderer()
    ],
    wrapper_class=structlog.make_filtering_bound_logger(20),  # INFO level
    logger_factory=structlog.WriteLoggerFactory(),
    cache_logger_on_first_use=True,
)

logger = structlog.get_logger()

# Usage in processors
await logger.ainfo("transcription_started", meeting_id=meeting_id, file_size=file_size)
```

### 3. Performance Monitoring

```python
# monitoring/metrics.py
import time
from contextlib import asynccontextmanager

@asynccontextmanager
async def track_processing_time(operation: str, meeting_id: str):
    start_time = time.time()
    try:
        yield
    finally:
        duration = time.time() - start_time
        await log_metric("processing_time", duration, {
            "operation": operation,
            "meeting_id": meeting_id
        })
```

This quickstart provides everything needed to begin developing and testing the meeting processing pipeline locally on a Mac M4 development environment.