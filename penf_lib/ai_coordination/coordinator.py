"""ModelCoordinator - Central component for multi-model AI coordination.

Manages parallel processing across multiple AI models, handles result aggregation,
and coordinates escalation decisions based on confidence scores and performance.
"""

import asyncio
import logging
import uuid
from datetime import datetime, timezone
from typing import Dict, List, Optional, Any, Set

from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select
from sqlalchemy.exc import SQLAlchemyError

from ..storage.events import EventPublisher
from ..storage.jobs import JobManager, JobStatus
from ..storage.models import ProcessingJob, ProcessingResult
from .ensemble import EnsembleCombiner, EnsembleResult
from .escalation import EscalationManager
from .performance import PerformanceTracker
from .models import CoordinationDecision, ModelProfile

logger = logging.getLogger(__name__)


class ModelCoordinator:
    """Central coordinator for multi-model AI processing workflows.

    Implements User Story 1: Parallel AI Processing Coordination
    - Coordinates multiple AI models working on same content
    - Manages model registration and subscription
    - Handles asynchronous processing with different completion times
    - Provides conflict resolution and failure handling
    """

    def __init__(
        self,
        session: AsyncSession,
        event_publisher: EventPublisher,
        job_manager: JobManager,
        ensemble_combiner: Optional[EnsembleCombiner] = None,
        escalation_manager: Optional[EscalationManager] = None,
        performance_tracker: Optional[PerformanceTracker] = None,
    ):
        self.session = session
        self.event_publisher = event_publisher
        self.job_manager = job_manager
        self.ensemble_combiner = ensemble_combiner or EnsembleCombiner()
        self.escalation_manager = escalation_manager or EscalationManager(session)
        self.performance_tracker = performance_tracker or PerformanceTracker(session)

        # Track active coordination workflows
        self.active_coordinations: Dict[str, Dict] = {}
        self.registered_models: Dict[str, ModelProfile] = {}

    async def register_model(
        self,
        model_id: str,
        model_profile: ModelProfile,
        event_types: List[str],
        content_filters: Optional[Dict[str, Any]] = None
    ) -> bool:
        """Register an AI model for coordinated processing.

        Args:
            model_id: Unique identifier for the model
            model_profile: Model capabilities and characteristics
            event_types: List of event types this model can process
            content_filters: Optional JSONB filters for content matching

        Returns:
            True if registration succeeded
        """
        try:
            # Store model profile
            self.registered_models[model_id] = model_profile

            # Subscribe model to relevant events through job manager
            subscription_id = await self.job_manager.subscription_manager.create_subscription(
                subscriber_id=model_id,
                subscription_name=f"{model_id}_coordination",
                event_channels=[f"{event_type}.processing" for event_type in event_types],
                event_types=event_types,
                filter_criteria=content_filters or {}
            )

            logger.info(f"Registered model {model_id} for coordination with subscription {subscription_id}")
            return True

        except Exception as e:
            logger.error(f"Failed to register model {model_id}: {e}")
            return False

    async def coordinate_processing(
        self,
        content_id: str,
        content_type: str,
        content_data: Dict[str, Any],
        target_models: Optional[List[str]] = None,
        coordination_timeout: int = 300,  # 5 minutes default
        require_minimum_models: int = 1
    ) -> str:
        """Coordinate multi-model processing for content.

        Implements parallel processing coordination with result aggregation.

        Args:
            content_id: Unique identifier for content being processed
            content_type: Type of content (email, document, meeting, etc.)
            content_data: Content payload for processing
            target_models: Specific models to use (if None, uses all applicable)
            coordination_timeout: Maximum time to wait for all models
            require_minimum_models: Minimum models required for success

        Returns:
            Coordination ID for tracking this workflow
        """
        coordination_id = str(uuid.uuid4())

        try:
            # Determine which models should process this content
            selected_models = await self._select_models_for_content(
                content_type, content_data, target_models
            )

            if len(selected_models) < require_minimum_models:
                raise ValueError(f"Only {len(selected_models)} models available, need {require_minimum_models}")

            # Create coordination workflow tracking
            self.active_coordinations[coordination_id] = {
                "content_id": content_id,
                "content_type": content_type,
                "content_data": content_data,
                "selected_models": selected_models,
                "started_at": datetime.now(timezone.utc),
                "timeout_at": datetime.now(timezone.utc).timestamp() + coordination_timeout,
                "job_ids": [],
                "completed_jobs": {},
                "failed_jobs": {},
                "status": "initializing"
            }

            # Publish content ingestion event to trigger model processing
            event_id = await self.event_publisher.publish(
                event_type="content.ingested",
                event_data={
                    "content_id": content_id,
                    "content_type": content_type,
                    "coordination_id": coordination_id,
                    "target_models": selected_models,
                    **content_data
                },
                channel=f"{content_type}.processing"
            )

            # Create processing jobs for each selected model
            job_ids = []
            for model_id in selected_models:
                job_id = await self.job_manager.create_job(
                    event_id=event_id,
                    processor_name=model_id,
                    job_type=f"{content_type}_processing",
                    input_data={
                        "content_id": content_id,
                        "content_type": content_type,
                        "coordination_id": coordination_id,
                        **content_data
                    },
                    priority=self.registered_models[model_id].priority,
                    metadata={"coordination_id": coordination_id}
                )
                job_ids.append(job_id)

            # Update coordination tracking
            self.active_coordinations[coordination_id].update({
                "job_ids": job_ids,
                "status": "processing",
                "event_id": event_id
            })

            logger.info(
                f"Started coordination {coordination_id} for {content_id} "
                f"with {len(selected_models)} models: {selected_models}"
            )

            return coordination_id

        except Exception as e:
            logger.error(f"Failed to coordinate processing for {content_id}: {e}")
            if coordination_id in self.active_coordinations:
                self.active_coordinations[coordination_id]["status"] = "failed"
                self.active_coordinations[coordination_id]["error"] = str(e)
            raise

    async def get_coordination_status(self, coordination_id: str) -> Optional[Dict[str, Any]]:
        """Get current status of a coordination workflow.

        Args:
            coordination_id: Coordination workflow ID

        Returns:
            Status information or None if not found
        """
        if coordination_id not in self.active_coordinations:
            return None

        coordination = self.active_coordinations[coordination_id]

        # Update job statuses
        await self._update_coordination_status(coordination_id)

        return {
            "coordination_id": coordination_id,
            "content_id": coordination["content_id"],
            "status": coordination["status"],
            "started_at": coordination["started_at"],
            "selected_models": coordination["selected_models"],
            "completed_jobs": len(coordination["completed_jobs"]),
            "failed_jobs": len(coordination["failed_jobs"]),
            "total_jobs": len(coordination["job_ids"]),
            "results_available": len(coordination["completed_jobs"]) > 0,
            "ensemble_ready": coordination.get("ensemble_result") is not None
        }

    async def wait_for_coordination(
        self,
        coordination_id: str,
        timeout: Optional[int] = None,
        min_results: int = 1
    ) -> Dict[str, Any]:
        """Wait for coordination to complete or reach minimum results.

        Args:
            coordination_id: Coordination workflow ID
            timeout: Maximum time to wait (uses coordination timeout if None)
            min_results: Minimum number of results before returning

        Returns:
            Coordination results including ensemble if available
        """
        if coordination_id not in self.active_coordinations:
            raise ValueError(f"Unknown coordination ID: {coordination_id}")

        coordination = self.active_coordinations[coordination_id]
        timeout_at = timeout or coordination["timeout_at"]

        while True:
            await self._update_coordination_status(coordination_id)

            # Check if we have enough results
            if len(coordination["completed_jobs"]) >= min_results:
                break

            # Check for timeout
            if datetime.now(timezone.utc).timestamp() > timeout_at:
                logger.warning(f"Coordination {coordination_id} timed out")
                coordination["status"] = "timeout"
                break

            # Check if all jobs failed
            if (len(coordination["failed_jobs"]) == len(coordination["job_ids"])
                and len(coordination["completed_jobs"]) == 0):
                logger.error(f"All jobs failed for coordination {coordination_id}")
                coordination["status"] = "failed"
                break

            # Wait before checking again
            await asyncio.sleep(1.0)

        # Create ensemble result if we have multiple completed jobs
        if len(coordination["completed_jobs"]) > 1:
            await self._create_ensemble_result(coordination_id)

        # Update performance tracking
        await self._update_performance_metrics(coordination_id)

        return await self._get_coordination_results(coordination_id)

    async def _select_models_for_content(
        self,
        content_type: str,
        content_data: Dict[str, Any],
        target_models: Optional[List[str]] = None
    ) -> List[str]:
        """Select appropriate models for content processing.

        Uses performance tracking and model profiles to choose optimal models.
        """
        if target_models:
            # Use specified models if provided
            available_models = [
                model_id for model_id in target_models
                if model_id in self.registered_models
            ]
        else:
            # Select all models that can handle this content type
            available_models = [
                model_id for model_id, profile in self.registered_models.items()
                if content_type in profile.supported_content_types
            ]

        # Use performance tracker to optimize model selection
        optimized_models = await self.performance_tracker.optimize_model_selection(
            content_type, available_models
        )

        return optimized_models

    async def _update_coordination_status(self, coordination_id: str) -> None:
        """Update coordination status by checking job progress."""
        coordination = self.active_coordinations[coordination_id]

        for job_id in coordination["job_ids"]:
            if job_id in coordination["completed_jobs"] or job_id in coordination["failed_jobs"]:
                continue

            # Get job status from job manager
            job = await self.job_manager.get_job(job_id)
            if not job:
                continue

            if job.status == JobStatus.COMPLETED:
                # Get job results
                results = await self.job_manager.get_job_results(job_id)
                coordination["completed_jobs"][job_id] = {
                    "processor_name": job.processor_name,
                    "completed_at": job.completed_at,
                    "results": results
                }

            elif job.status in [JobStatus.FAILED, JobStatus.CANCELLED]:
                coordination["failed_jobs"][job_id] = {
                    "processor_name": job.processor_name,
                    "failed_at": job.failed_at or job.updated_at,
                    "error": job.error_message
                }

    async def _create_ensemble_result(self, coordination_id: str) -> None:
        """Create ensemble result from completed jobs."""
        coordination = self.active_coordinations[coordination_id]

        if len(coordination["completed_jobs"]) < 2:
            return

        # Collect results from completed jobs
        individual_results = []
        for job_id, job_data in coordination["completed_jobs"].items():
            for result in job_data["results"]:
                individual_results.append({
                    "processor_name": job_data["processor_name"],
                    "result_data": result.result_data,
                    "confidence_score": result.confidence_score,
                    "model_name": result.model_name,
                    "processing_time_ms": result.processing_time_ms
                })

        # Use ensemble combiner to create aggregated result
        ensemble_result = await self.ensemble_combiner.combine_results(
            individual_results,
            coordination["content_type"],
            coordination["content_data"]
        )

        coordination["ensemble_result"] = ensemble_result

    async def _update_performance_metrics(self, coordination_id: str) -> None:
        """Update performance metrics for all models involved."""
        coordination = self.active_coordinations[coordination_id]

        for job_id, job_data in coordination["completed_jobs"].items():
            await self.performance_tracker.record_performance(
                model_id=job_data["processor_name"],
                content_type=coordination["content_type"],
                processing_time=job_data["results"][0].processing_time_ms if job_data["results"] else None,
                confidence_score=job_data["results"][0].confidence_score if job_data["results"] else None,
                success=True
            )

        for job_id, job_data in coordination["failed_jobs"].items():
            await self.performance_tracker.record_performance(
                model_id=job_data["processor_name"],
                content_type=coordination["content_type"],
                processing_time=None,
                confidence_score=None,
                success=False
            )

    async def _get_coordination_results(self, coordination_id: str) -> Dict[str, Any]:
        """Get comprehensive coordination results."""
        coordination = self.active_coordinations[coordination_id]

        return {
            "coordination_id": coordination_id,
            "content_id": coordination["content_id"],
            "content_type": coordination["content_type"],
            "status": coordination["status"],
            "started_at": coordination["started_at"],
            "completed_at": datetime.now(timezone.utc),
            "selected_models": coordination["selected_models"],
            "individual_results": coordination["completed_jobs"],
            "failed_jobs": coordination["failed_jobs"],
            "ensemble_result": coordination.get("ensemble_result"),
            "performance_summary": {
                "total_models": len(coordination["selected_models"]),
                "successful_models": len(coordination["completed_jobs"]),
                "failed_models": len(coordination["failed_jobs"]),
                "completion_rate": len(coordination["completed_jobs"]) / len(coordination["selected_models"])
            }
        }