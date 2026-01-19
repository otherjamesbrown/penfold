"""
Database connection and model definitions

Handles PostgreSQL connection with async SQLAlchemy and pgvector
extension for semantic search capabilities.
"""
from sqlalchemy.ext.asyncio import create_async_engine, AsyncSession, async_sessionmaker
from sqlalchemy.orm import DeclarativeBase, mapped_column, Mapped
from sqlalchemy.dialects.postgresql import UUID, JSONB
from sqlalchemy import String, Integer, Float, Boolean, DateTime, LargeBinary, Text
from sqlalchemy.sql import func
from pgvector.sqlalchemy import Vector
import uuid
from datetime import datetime
from typing import Optional, Dict, Any, AsyncGenerator

from app.config import get_database_url


class Base(DeclarativeBase):
    """Base class for all database models"""
    pass


# Database engine and session management
engine = create_async_engine(
    get_database_url(),
    echo=True,  # Set to False in production
    pool_size=20,
    max_overflow=30,
    pool_pre_ping=True,
)

async_session = async_sessionmaker(engine, class_=AsyncSession)


async def get_db() -> AsyncGenerator[AsyncSession, None]:
    """Dependency to get database session"""
    async with async_session() as session:
        try:
            yield session
        finally:
            await session.close()


async def init_database():
    """Initialize database tables and extensions"""
    async with engine.begin() as conn:
        # Create pgvector extension
        await conn.execute("CREATE EXTENSION IF NOT EXISTS vector")

        # Create all tables
        await conn.run_sync(Base.metadata.create_all)

        print("✅ Database tables and extensions created")


# Core Models (from data-model.md specification)

class MeetingFile(Base):
    """Uploaded meeting recordings and documents"""
    __tablename__ = "meeting_files"

    id: Mapped[uuid.UUID] = mapped_column(UUID(as_uuid=True), primary_key=True, default=uuid.uuid4)
    filename: Mapped[str] = mapped_column(String(255), nullable=False)
    size_bytes: Mapped[int] = mapped_column(Integer, nullable=False)
    privacy_level: Mapped[str] = mapped_column(String(20), nullable=False)  # public, organization, team_only, confidential
    storage_location: Mapped[str] = mapped_column(String(50), nullable=False)  # local, cloud, hybrid
    encryption_key_id: Mapped[Optional[str]] = mapped_column(String(100), nullable=True)
    upload_completed_at: Mapped[Optional[datetime]] = mapped_column(DateTime(timezone=True), nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=func.now())
    metadata: Mapped[Optional[Dict[str, Any]]] = mapped_column(JSONB, nullable=True)


class ProcessingJob(Base):
    """Background processing jobs for meeting files"""
    __tablename__ = "processing_jobs"

    id: Mapped[uuid.UUID] = mapped_column(UUID(as_uuid=True), primary_key=True, default=uuid.uuid4)
    meeting_file_id: Mapped[uuid.UUID] = mapped_column(UUID(as_uuid=True), nullable=False)
    job_type: Mapped[str] = mapped_column(String(50), nullable=False)  # upload, transcription, analysis, embedding_generation
    status: Mapped[str] = mapped_column(String(20), nullable=False)  # queued, processing, completed, failed, dead_letter
    progress: Mapped[int] = mapped_column(Integer, default=0)  # 0-100 percentage
    stage: Mapped[Optional[str]] = mapped_column(String(100), nullable=True)
    estimated_completion_time: Mapped[Optional[datetime]] = mapped_column(DateTime(timezone=True), nullable=True)
    started_at: Mapped[Optional[datetime]] = mapped_column(DateTime(timezone=True), nullable=True)
    completed_at: Mapped[Optional[datetime]] = mapped_column(DateTime(timezone=True), nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=func.now())
    error_details: Mapped[Optional[Dict[str, Any]]] = mapped_column(JSONB, nullable=True)
    idempotency_key: Mapped[str] = mapped_column(String(100), nullable=False)


class MeetingTranscript(Base):
    """Transcribed text from audio/video meetings"""
    __tablename__ = "meeting_transcripts"

    id: Mapped[uuid.UUID] = mapped_column(UUID(as_uuid=True), primary_key=True, default=uuid.uuid4)
    meeting_file_id: Mapped[uuid.UUID] = mapped_column(UUID(as_uuid=True), nullable=False, unique=True)
    transcript_text: Mapped[str] = mapped_column(Text, nullable=False)
    embedding: Mapped[Optional[list]] = mapped_column(Vector(1536), nullable=True)  # OpenAI embedding dimension
    confidence_score: Mapped[float] = mapped_column(Float, nullable=False)
    transcription_method: Mapped[str] = mapped_column(String(50), nullable=False)  # whisper_local, google_cloud, manual
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=func.now())
    speaker_segments: Mapped[Optional[Dict[str, Any]]] = mapped_column(JSONB, nullable=True)
    quality_metrics: Mapped[Optional[Dict[str, Any]]] = mapped_column(JSONB, nullable=True)


class MeetingParticipant(Base):
    """Meeting participants with speaker identification"""
    __tablename__ = "meeting_participants"

    id: Mapped[uuid.UUID] = mapped_column(UUID(as_uuid=True), primary_key=True, default=uuid.uuid4)
    meeting_file_id: Mapped[uuid.UUID] = mapped_column(UUID(as_uuid=True), nullable=False)
    person_id: Mapped[Optional[uuid.UUID]] = mapped_column(UUID(as_uuid=True), nullable=True)  # null for provisional
    speaker_label: Mapped[str] = mapped_column(String(50), nullable=False)  # "Speaker 1", "Speaker 2", etc.
    voice_signature_hash: Mapped[str] = mapped_column(String(64), nullable=False)
    identification_confidence: Mapped[Optional[float]] = mapped_column(Float, nullable=True)
    is_provisional: Mapped[bool] = mapped_column(Boolean, default=True)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=func.now())
    contribution_stats: Mapped[Optional[Dict[str, Any]]] = mapped_column(JSONB, nullable=True)


class MeetingSummary(Base):
    """AI-generated summaries and key insights from meetings"""
    __tablename__ = "meeting_summaries"

    id: Mapped[uuid.UUID] = mapped_column(UUID(as_uuid=True), primary_key=True, default=uuid.uuid4)
    meeting_file_id: Mapped[uuid.UUID] = mapped_column(UUID(as_uuid=True), nullable=False, unique=True)
    summary_text: Mapped[str] = mapped_column(Text, nullable=False)
    summary_embedding: Mapped[Optional[list]] = mapped_column(Vector(1536), nullable=True)
    key_points: Mapped[Optional[Dict[str, Any]]] = mapped_column(JSONB, nullable=True)
    action_items: Mapped[Optional[Dict[str, Any]]] = mapped_column(JSONB, nullable=True)
    decisions: Mapped[Optional[Dict[str, Any]]] = mapped_column(JSONB, nullable=True)
    quality_score: Mapped[float] = mapped_column(Float, nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=func.now())


class MeetingTopic(Base):
    """Identified topics and project links from meetings"""
    __tablename__ = "meeting_topics"

    id: Mapped[uuid.UUID] = mapped_column(UUID(as_uuid=True), primary_key=True, default=uuid.uuid4)
    meeting_file_id: Mapped[uuid.UUID] = mapped_column(UUID(as_uuid=True), nullable=False)
    project_id: Mapped[Optional[uuid.UUID]] = mapped_column(UUID(as_uuid=True), nullable=True)
    topic_name: Mapped[str] = mapped_column(String(200), nullable=False)
    relevance_score: Mapped[float] = mapped_column(Float, nullable=False)
    topic_category: Mapped[str] = mapped_column(String(100), nullable=False)
    first_mentioned: Mapped[Optional[datetime]] = mapped_column(DateTime(timezone=True), nullable=True)
    last_mentioned: Mapped[Optional[datetime]] = mapped_column(DateTime(timezone=True), nullable=True)
    context_snippets: Mapped[Optional[Dict[str, Any]]] = mapped_column(JSONB, nullable=True)


class ReviewQueue(Base):
    """Manual review tasks for failed automatic processing"""
    __tablename__ = "review_queue"

    id: Mapped[uuid.UUID] = mapped_column(UUID(as_uuid=True), primary_key=True, default=uuid.uuid4)
    meeting_file_id: Mapped[uuid.UUID] = mapped_column(UUID(as_uuid=True), nullable=False)
    review_type: Mapped[str] = mapped_column(String(50), nullable=False)  # speaker_identification, project_linking, transcription_quality
    status: Mapped[str] = mapped_column(String(20), nullable=False)  # pending, in_review, completed, escalated
    ai_suggestions: Mapped[Optional[Dict[str, Any]]] = mapped_column(JSONB, nullable=True)
    user_feedback: Mapped[Optional[Dict[str, Any]]] = mapped_column(JSONB, nullable=True)
    assigned_to: Mapped[Optional[uuid.UUID]] = mapped_column(UUID(as_uuid=True), nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=func.now())
    resolved_at: Mapped[Optional[datetime]] = mapped_column(DateTime(timezone=True), nullable=True)
    resolution_notes: Mapped[Optional[str]] = mapped_column(Text, nullable=True)


class MeetingInsight(Base):
    """Extracted business insights from meetings"""
    __tablename__ = "meeting_insights"

    id: Mapped[uuid.UUID] = mapped_column(UUID(as_uuid=True), primary_key=True, default=uuid.uuid4)
    meeting_file_id: Mapped[uuid.UUID] = mapped_column(UUID(as_uuid=True), nullable=False)
    insight_type: Mapped[str] = mapped_column(String(50), nullable=False)
    insight_text: Mapped[str] = mapped_column(Text, nullable=False)
    confidence_score: Mapped[float] = mapped_column(Float, nullable=False)
    supporting_evidence: Mapped[Optional[Dict[str, Any]]] = mapped_column(JSONB, nullable=True)
    related_entity_id: Mapped[Optional[uuid.UUID]] = mapped_column(UUID(as_uuid=True), nullable=True)
    related_entity_type: Mapped[Optional[str]] = mapped_column(String(50), nullable=True)  # person, project, topic
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=func.now())