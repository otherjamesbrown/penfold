"""Job management service with state machine and retry logic.

This module provides job lifecycle management for processing events
through various AI models and processors with proper state tracking.
"""

import logging
from typing import Dict, Any, List, Optional, Union
from datetime import datetime, timezone, timedelta
from enum import Enum
import uuid

from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, update, and_, or_, text, func
from sqlalchemy.exc import SQLAlchemyError

from penf_lib.storage.models import ProcessingJob, ProcessingResult


logger = logging.getLogger(__name__)


class JobStatus(str, Enum):
    """Job status enumeration for state machine."""
    PENDING = "pending"
    RUNNING = "running"
    COMPLETED = "completed"
    FAILED = "failed"
    CANCELLED = "cancelled"
    RETRYING = "retrying"


class JobManager:
    """Manages processing job lifecycle with state machine and retry logic.

    Provides comprehensive job management including creation, state transitions,
    result storage, and retry handling for robust event processing.
    """

    def __init__(self, session: AsyncSession, max_job_age_days: int = 30):
        """Initialize job manager.

        Args:
            session: Database session for job persistence
            max_job_age_days: Maximum age for job retention
        """
        self.session = session
        self.max_job_age_days = max_job_age_days

    async def create_job(
        self,
        event_id: str,
        processor_type: str,
        input_data: Dict[str, Any],
        priority: int = 5,
        max_retries: int = 3,
        timeout_seconds: int = 300,
        scheduled_at: Optional[datetime] = None,
    ) -> str:
        """Create a new processing job.

        Args:
            event_id: ID of the triggering event
            processor_type: Type of processor to handle this job
            input_data: Input data for processing
            priority: Job priority (1=highest, 10=lowest)
            max_retries: Maximum retry attempts
            timeout_seconds: Job timeout in seconds
            scheduled_at: Optional delayed execution time

        Returns:
            Job ID for tracking

        Raises:
            ValueError: Invalid job parameters
            RuntimeError: Job creation failed
        """
        if not event_id or not processor_type or not isinstance(input_data, dict):
            raise ValueError("Event ID, processor type, and input data are required")

        job_id = str(uuid.uuid4())
        current_time = datetime.now(timezone.utc)

        job = ProcessingJob(
            id=job_id,
            event_id=event_id,
            processor_type=processor_type,
            status=JobStatus.PENDING,
            priority=priority,
            input_data=input_data,
            max_retries=max_retries,
            retry_count=0,
            timeout_seconds=timeout_seconds,
            scheduled_at=scheduled_at or current_time,
            created_at=current_time,
        )

        try:
            self.session.add(job)
            await self.session.commit()
            logger.info(f"Created job {job_id} for event {event_id} with processor {processor_type}")
            return job_id

        except SQLAlchemyError as e:
            await self.session.rollback()
            logger.error(f"Failed to create job {job_id}: {e}")
            raise RuntimeError(f"Job creation failed: {e}")

    async def get_job(self, job_id: str) -> Optional[ProcessingJob]:
        """Retrieve a job by ID.

        Args:
            job_id: Job identifier

        Returns:
            Job object or None if not found
        """
        try:
            result = await self.session.execute(
                select(ProcessingJob).where(ProcessingJob.id == job_id)
            )
            return result.scalar_one_or_none()

        except SQLAlchemyError as e:
            logger.error(f"Failed to get job {job_id}: {e}")
            return None

    async def start_job(self, job_id: str, worker_id: Optional[str] = None) -> bool:
        """Start a pending job.

        Args:
            job_id: Job to start
            worker_id: Optional worker identifier

        Returns:
            True if job was started successfully

        Raises:
            ValueError: Invalid state transition
        """
        job = await self.get_job(job_id)
        if not job:
            logger.warning(f"Job {job_id} not found")
            return False

        if job.status != JobStatus.PENDING:
            raise ValueError(f"Cannot start job {job_id} in status {job.status}")

        try:
            await self.session.execute(
                update(ProcessingJob)
                .where(ProcessingJob.id == job_id)
                .values(
                    status=JobStatus.RUNNING,
                    started_at=datetime.now(timezone.utc),
                    worker_id=worker_id,
                )
            )
            await self.session.commit()
            logger.info(f"Started job {job_id}")
            return True

        except SQLAlchemyError as e:
            await self.session.rollback()
            logger.error(f"Failed to start job {job_id}: {e}")
            return False

    async def complete_job(
        self,
        job_id: str,
        result_data: Dict[str, Any],
        result_type: str = "default",
        model_name: Optional[str] = None,
        model_version: Optional[str] = None,
    ) -> bool:
        """Complete a job with results.

        Args:
            job_id: Job to complete
            result_data: Processing results
            result_type: Type of result
            model_name: Model that produced the result
            model_version: Version of the model

        Returns:
            True if job was completed successfully
        """
        job = await self.get_job(job_id)
        if not job:
            logger.warning(f"Job {job_id} not found")
            return False

        if job.status != JobStatus.RUNNING:
            logger.warning(f"Cannot complete job {job_id} in status {job.status}")
            return False

        current_time = datetime.now(timezone.utc)

        try:
            # Update job status
            await self.session.execute(
                update(ProcessingJob)
                .where(ProcessingJob.id == job_id)
                .values(
                    status=JobStatus.COMPLETED,
                    completed_at=current_time,
                    execution_time_seconds=func.extract(
                        "epoch",
                        current_time - ProcessingJob.started_at
                    ),
                )
            )

            # Store results
            result = ProcessingResult(
                job_id=job_id,
                result_type=result_type,
                result_data=result_data,
                model_name=model_name,
                model_version=model_version,
                confidence_score=result_data.get("confidence", 1.0),
                validation_status="pending",
            )
            self.session.add(result)

            await self.session.commit()
            logger.info(f"Completed job {job_id} with results")
            return True

        except SQLAlchemyError as e:
            await self.session.rollback()
            logger.error(f"Failed to complete job {job_id}: {e}")
            return False

    async def fail_job(
        self,
        job_id: str,
        error_message: str,
        error_details: Optional[Dict[str, Any]] = None,
    ) -> bool:
        """Mark a job as failed.

        Args:
            job_id: Job that failed
            error_message: Error description
            error_details: Optional detailed error information

        Returns:
            True if job was marked as failed successfully
        """
        job = await self.get_job(job_id)
        if not job:
            logger.warning(f"Job {job_id} not found")
            return False

        current_time = datetime.now(timezone.utc)

        try:
            await self.session.execute(
                update(ProcessingJob)
                .where(ProcessingJob.id == job_id)
                .values(
                    status=JobStatus.FAILED,
                    completed_at=current_time,
                    error_message=error_message,
                    error_details=error_details or {},
                    execution_time_seconds=func.extract(
                        "epoch",
                        current_time - ProcessingJob.started_at
                    ) if job.started_at else None,
                )
            )
            await self.session.commit()
            logger.info(f"Marked job {job_id} as failed: {error_message}")
            return True

        except SQLAlchemyError as e:
            await self.session.rollback()
            logger.error(f"Failed to mark job {job_id} as failed: {e}")
            return False

    async def retry_job(self, job_id: str) -> Optional[str]:
        """Retry a failed job by creating a new job instance.

        Args:
            job_id: Failed job to retry

        Returns:
            New job ID if retry was created, None otherwise
        """
        job = await self.get_job(job_id)
        if not job:
            logger.warning(f"Job {job_id} not found")
            return None

        if job.status != JobStatus.FAILED:
            logger.warning(f"Cannot retry job {job_id} in status {job.status}")
            return None

        if job.retry_count >= job.max_retries:
            logger.warning(f"Job {job_id} has exceeded maximum retries ({job.max_retries})")
            return None

        try:
            # Create a new job with incremented retry count
            new_job_id = await self.create_job(
                event_id=job.event_id,
                processor_type=job.processor_type,
                input_data=job.input_data,
                priority=job.priority,
                max_retries=job.max_retries,
                timeout_seconds=job.timeout_seconds,
            )

            # Update retry count on the new job
            await self.session.execute(
                update(ProcessingJob)
                .where(ProcessingJob.id == new_job_id)
                .values(
                    retry_count=job.retry_count + 1,
                    original_job_id=job_id,
                )
            )

            # Mark original job as retrying
            await self.session.execute(
                update(ProcessingJob)
                .where(ProcessingJob.id == job_id)
                .values(status=JobStatus.RETRYING)
            )

            await self.session.commit()
            logger.info(f"Created retry job {new_job_id} for failed job {job_id}")
            return new_job_id

        except Exception as e:
            await self.session.rollback()
            logger.error(f"Failed to create retry job for {job_id}: {e}")
            return None

    async def cancel_job(self, job_id: str, reason: Optional[str] = None) -> bool:
        """Cancel a pending or running job.

        Args:
            job_id: Job to cancel
            reason: Optional cancellation reason

        Returns:
            True if job was cancelled successfully
        """
        job = await self.get_job(job_id)
        if not job:
            logger.warning(f"Job {job_id} not found")
            return False

        if job.status in [JobStatus.COMPLETED, JobStatus.CANCELLED]:
            logger.warning(f"Cannot cancel job {job_id} in status {job.status}")
            return False

        try:
            await self.session.execute(
                update(ProcessingJob)
                .where(ProcessingJob.id == job_id)
                .values(
                    status=JobStatus.CANCELLED,
                    completed_at=datetime.now(timezone.utc),
                    error_message=reason or "Job cancelled",
                )
            )
            await self.session.commit()
            logger.info(f"Cancelled job {job_id}: {reason}")
            return True

        except SQLAlchemyError as e:
            await self.session.rollback()
            logger.error(f"Failed to cancel job {job_id}: {e}")
            return False

    async def get_pending_jobs(
        self,
        processor_type: Optional[str] = None,
        priority_threshold: Optional[int] = None,
        limit: int = 100,
    ) -> List[ProcessingJob]:
        """Get pending jobs ready for processing.

        Args:
            processor_type: Optional filter by processor type
            priority_threshold: Optional minimum priority (lower numbers = higher priority)
            limit: Maximum number of jobs to return

        Returns:
            List of pending jobs ordered by priority and created time
        """
        current_time = datetime.now(timezone.utc)

        query = (
            select(ProcessingJob)
            .where(
                and_(
                    ProcessingJob.status == JobStatus.PENDING,
                    ProcessingJob.scheduled_at <= current_time,
                )
            )
            .order_by(ProcessingJob.priority, ProcessingJob.created_at)
            .limit(limit)
        )

        if processor_type:
            query = query.where(ProcessingJob.processor_type == processor_type)

        if priority_threshold is not None:
            query = query.where(ProcessingJob.priority <= priority_threshold)

        try:
            result = await self.session.execute(query)
            return result.scalars().all()

        except SQLAlchemyError as e:
            logger.error(f"Failed to get pending jobs: {e}")
            return []

    async def get_job_results(self, job_id: str) -> List[ProcessingResult]:
        """Get all results for a job.

        Args:
            job_id: Job ID to get results for

        Returns:
            List of processing results
        """
        try:
            result = await self.session.execute(
                select(ProcessingResult)
                .where(ProcessingResult.job_id == job_id)
                .order_by(ProcessingResult.created_at)
            )
            return result.scalars().all()

        except SQLAlchemyError as e:
            logger.error(f"Failed to get job results for {job_id}: {e}")
            return []

    async def get_job_statistics(
        self,
        processor_type: Optional[str] = None,
        hours: int = 24,
    ) -> Dict[str, Any]:
        """Get job processing statistics.

        Args:
            processor_type: Optional filter by processor type
            hours: Number of hours to include in statistics

        Returns:
            Dictionary of job statistics
        """
        cutoff_time = datetime.now(timezone.utc) - timedelta(hours=hours)

        query = select(
            ProcessingJob.status,
            func.count().label("count"),
            func.avg(ProcessingJob.execution_time_seconds).label("avg_execution_time"),
            func.max(ProcessingJob.execution_time_seconds).label("max_execution_time"),
        ).where(ProcessingJob.created_at >= cutoff_time)

        if processor_type:
            query = query.where(ProcessingJob.processor_type == processor_type)

        query = query.group_by(ProcessingJob.status)

        try:
            result = await self.session.execute(query)
            stats = {}
            total_jobs = 0

            for row in result:
                stats[row.status] = {
                    "count": row.count,
                    "avg_execution_time": float(row.avg_execution_time or 0),
                    "max_execution_time": float(row.max_execution_time or 0),
                }
                total_jobs += row.count

            stats["total"] = total_jobs
            stats["success_rate"] = (
                stats.get("completed", {}).get("count", 0) / total_jobs * 100
                if total_jobs > 0 else 0
            )

            return stats

        except SQLAlchemyError as e:
            logger.error(f"Failed to get job statistics: {e}")
            return {}

    async def cleanup_old_jobs(self, dry_run: bool = True) -> Dict[str, int]:
        """Clean up old completed and failed jobs.

        Args:
            dry_run: If True, only count what would be deleted

        Returns:
            Dictionary with cleanup statistics
        """
        cutoff_time = datetime.now(timezone.utc) - timedelta(days=self.max_job_age_days)

        # Count jobs to be cleaned up
        count_query = (
            select(func.count())
            .where(
                and_(
                    ProcessingJob.created_at < cutoff_time,
                    ProcessingJob.status.in_([JobStatus.COMPLETED, JobStatus.FAILED, JobStatus.CANCELLED]),
                )
            )
        )

        try:
            result = await self.session.execute(count_query)
            job_count = result.scalar()

            if dry_run:
                return {"would_delete_jobs": job_count}

            # Delete old jobs and their results (CASCADE should handle results)
            delete_query = (
                update(ProcessingJob)
                .where(
                    and_(
                        ProcessingJob.created_at < cutoff_time,
                        ProcessingJob.status.in_([JobStatus.COMPLETED, JobStatus.FAILED, JobStatus.CANCELLED]),
                    )
                )
                .values(deleted_at=datetime.now(timezone.utc))
            )

            await self.session.execute(delete_query)
            await self.session.commit()

            logger.info(f"Cleaned up {job_count} old jobs")
            return {"deleted_jobs": job_count}

        except SQLAlchemyError as e:
            await self.session.rollback()
            logger.error(f"Failed to cleanup old jobs: {e}")
            return {"error": str(e)}