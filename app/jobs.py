"""
Job queue configuration using Procrastinate

Provides background job processing for meeting file uploads, transcription,
analysis, and other long-running tasks with PostgreSQL-based persistence.
"""
import procrastinate
from sqlalchemy.ext.asyncio import create_async_engine
import asyncio
from typing import Dict, Any, Optional
import logging

from app.config import get_database_url, settings

# Create job app with PostgreSQL connector
job_app = procrastinate.App(
    connector=procrastinate.AiopgConnector(
        get_database_url().replace("postgresql+asyncpg://", "postgresql://")
    )
)

# Configure logging
logger = logging.getLogger(__name__)


async def init_job_queue():
    """Initialize job queue schema and worker"""
    try:
        # Apply procrastinate schema
        engine = create_async_engine(get_database_url())
        async with engine.begin() as conn:
            # Procrastinate needs sync connection for schema operations
            await conn.run_sync(
                lambda sync_conn: procrastinate.schema.apply_schema(
                    sync_conn, schema=procrastinate.schema.SchemaManager()
                )
            )

        logger.info("✅ Job queue schema initialized")

        # Initialize transcription job queue
        from app.transcription.jobs import init_job_queue as init_transcription_jobs
        await init_transcription_jobs()

    except Exception as e:
        logger.error(f"❌ Failed to initialize job queue: {e}")
        raise


# Job definitions (will be implemented in respective modules)

@job_app.task(queue="upload", priority=5)
async def process_file_upload(meeting_file_id: str, upload_session_id: str):
    """Process completed file upload and initiate transcription"""
    # Will be implemented in app/upload/jobs.py
    logger.info(f"Processing file upload: {meeting_file_id}")
    return {"status": "upload_processed", "meeting_file_id": meeting_file_id}


@job_app.task(queue="transcription", priority=10)
async def transcribe_meeting(meeting_file_id: str, file_path: str, privacy_level: str):
    """Transcribe meeting audio with speaker identification"""
    # Will be implemented in app/transcription/jobs.py
    logger.info(f"Transcribing meeting: {meeting_file_id}")
    return {"status": "transcription_completed", "meeting_file_id": meeting_file_id}


@job_app.task(queue="analysis", priority=15)
async def analyze_meeting(meeting_file_id: str, transcript_id: str):
    """Generate summary and extract insights from transcript"""
    # Will be implemented in app/analysis/jobs.py
    logger.info(f"Analyzing meeting: {meeting_file_id}")
    return {"status": "analysis_completed", "meeting_file_id": meeting_file_id}


@job_app.task(queue="embedding", priority=20)
async def generate_embeddings(meeting_file_id: str, content_type: str):
    """Generate vector embeddings for semantic search"""
    # Will be implemented in app/analysis/embeddings.py
    logger.info(f"Generating embeddings: {meeting_file_id}")
    return {"status": "embeddings_generated", "meeting_file_id": meeting_file_id}


# Job management utilities

async def enqueue_meeting_processing_pipeline(
    meeting_file_id: str,
    file_path: str,
    privacy_level: str,
    upload_session_id: str
) -> Dict[str, Any]:
    """
    Enqueue complete meeting processing pipeline

    Creates a chain of dependent jobs:
    1. Process upload completion
    2. Transcribe audio
    3. Analyze transcript
    4. Generate embeddings
    """
    try:
        # Start with upload processing
        upload_job = await process_file_upload.defer_async(
            meeting_file_id=meeting_file_id,
            upload_session_id=upload_session_id
        )

        # Chain transcription job
        transcription_job = await transcribe_meeting.defer_async(
            meeting_file_id=meeting_file_id,
            file_path=file_path,
            privacy_level=privacy_level
        )

        # Chain analysis job
        analysis_job = await analyze_meeting.defer_async(
            meeting_file_id=meeting_file_id,
            transcript_id=f"transcript_{meeting_file_id}"
        )

        # Chain embedding generation
        embedding_job = await generate_embeddings.defer_async(
            meeting_file_id=meeting_file_id,
            content_type="transcript_and_summary"
        )

        return {
            "pipeline_started": True,
            "jobs": {
                "upload": upload_job.id,
                "transcription": transcription_job.id,
                "analysis": analysis_job.id,
                "embeddings": embedding_job.id
            }
        }

    except Exception as e:
        logger.error(f"Failed to enqueue processing pipeline: {e}")
        raise


async def get_job_status(job_id: str) -> Dict[str, Any]:
    """Get status of a specific job"""
    try:
        # This will be implemented when we integrate with Procrastinate's job store
        return {
            "job_id": job_id,
            "status": "queued",
            "progress": 0,
            "stage": "initializing"
        }
    except Exception as e:
        logger.error(f"Failed to get job status for {job_id}: {e}")
        return {"error": str(e)}


async def get_meeting_processing_status(meeting_file_id: str) -> Dict[str, Any]:
    """Get overall processing status for a meeting file"""
    try:
        # This will query the ProcessingJob table
        return {
            "meeting_file_id": meeting_file_id,
            "overall_status": "processing",
            "stages": {
                "upload": {"status": "completed", "progress": 100},
                "transcription": {"status": "processing", "progress": 45},
                "analysis": {"status": "queued", "progress": 0},
                "embeddings": {"status": "queued", "progress": 0}
            }
        }
    except Exception as e:
        logger.error(f"Failed to get meeting status for {meeting_file_id}: {e}")
        return {"error": str(e)}


# Worker management functions

async def start_worker():
    """Start procrastinate worker for processing jobs"""
    try:
        worker = procrastinate.Worker(
            app=job_app,
            concurrency=settings.MAX_CONCURRENT_JOBS,
            name="meeting-pipeline-worker"
        )

        logger.info(f"🚀 Starting worker with {settings.MAX_CONCURRENT_JOBS} max concurrent jobs")
        await worker.run_async()

    except Exception as e:
        logger.error(f"❌ Worker failed: {e}")
        raise


def run_worker():
    """Synchronous wrapper to run worker"""
    asyncio.run(start_worker())