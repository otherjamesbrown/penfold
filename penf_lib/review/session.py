"""Session management for the Daily Review workflow.

This module provides session lifecycle management including creation,
state transitions, and progress tracking.
"""

from __future__ import annotations

from datetime import datetime, timedelta, timezone
from typing import Any, Protocol
from uuid import UUID, uuid4

from penf_lib.review.exceptions import (
    ActiveSessionExistsError,
    InvalidSessionStateError,
    PendingItemsError,
    SessionNotFoundError,
)
from penf_lib.review.models import (
    PriorityMode,
    ReviewMode,
    SessionDTO,
    SessionStatus,
)


class ReviewRepositoryProtocol(Protocol):
    """Protocol for repository dependency injection.

    Defines the interface that any repository implementation must satisfy
    to work with the SessionManager.
    """

    async def create_session(self, session_data: dict[str, Any]) -> Any:
        """Create a new session in the database.

        Args:
            session_data: Session data dictionary

        Returns:
            Created session model
        """
        ...

    async def get_session_by_id(self, session_id: int) -> Any | None:
        """Get session by ID.

        Args:
            session_id: Session identifier

        Returns:
            Session model or None if not found
        """
        ...

    async def get_active_session_for_user(
        self,
        tenant_id: UUID,
        user_email: str,
    ) -> Any | None:
        """Get active session for a user.

        Args:
            tenant_id: Tenant isolation identifier
            user_email: User's email address

        Returns:
            Active session model or None
        """
        ...

    async def update_session(self, session: Any) -> Any:
        """Update session in database.

        Args:
            session: Session model to update

        Returns:
            Updated session model
        """
        ...


class SessionManager:
    """Manages review session lifecycle.

    The SessionManager is responsible for:
    - Creating new review sessions
    - Managing session state transitions (active/paused/completed/abandoned)
    - Tracking session progress
    - Resuming existing sessions
    """

    DEFAULT_SESSION_DURATION_HOURS = 24

    def __init__(self, repository: ReviewRepositoryProtocol) -> None:
        """Initialize the SessionManager.

        Args:
            repository: Repository implementing ReviewRepositoryProtocol
        """
        self._repository = repository

    async def create_session(
        self,
        tenant_id: UUID,
        user_email: str,
        total_items: int,
        mode: ReviewMode = ReviewMode.STANDARD,
        priority: PriorityMode = PriorityMode.MIXED,
        filter_criteria: dict[str, Any] | None = None,
    ) -> SessionDTO:
        """Create a new review session.

        Checks for existing active sessions and creates a new one if none exists.

        Args:
            tenant_id: Tenant isolation identifier
            user_email: User performing the review
            total_items: Total number of items in the queue
            mode: Review mode (quick, standard, detailed)
            priority: Priority mode for queue ordering
            filter_criteria: Optional filter criteria applied to queue

        Returns:
            Created session as DTO

        Raises:
            ActiveSessionExistsError: If user already has an active session
        """
        # Check for existing active session
        existing = await self._repository.get_active_session_for_user(
            tenant_id, user_email
        )
        if existing is not None:
            raise ActiveSessionExistsError(
                existing_session_id=existing.id,
                user_email=user_email,
            )

        # Create new session
        now = datetime.now(timezone.utc)
        session_data = {
            "session_uuid": uuid4(),
            "tenant_id": tenant_id,
            "user_email": user_email,
            "status": SessionStatus.ACTIVE,
            "started_at": now,
            "last_activity_at": now,
            "expires_at": now + timedelta(hours=self.DEFAULT_SESSION_DURATION_HOURS),
            "current_position": 0,
            "total_items": total_items,
            "items_reviewed": 0,
            "items_accepted": 0,
            "items_rejected": 0,
            "items_modified": 0,
            "items_skipped": 0,
            "review_mode": mode,
            "priority_mode": priority,
            "filter_criteria": filter_criteria or {},
            "session_metadata": {},
            "created_at": now,
            "updated_at": now,
        }

        created = await self._repository.create_session(session_data)
        return self._model_to_dto(created)

    async def get_active_session(
        self,
        tenant_id: UUID,
        user_email: str,
    ) -> SessionDTO | None:
        """Get active session for user, if any.

        Args:
            tenant_id: Tenant isolation identifier
            user_email: User's email address

        Returns:
            Active session DTO or None if no active session exists
        """
        session = await self._repository.get_active_session_for_user(
            tenant_id, user_email
        )
        if session is None:
            return None
        return self._model_to_dto(session)

    async def resume_session(self, session_id: int) -> SessionDTO:
        """Resume a paused or active session.

        Updates the last_activity_at timestamp and sets status to active
        if currently paused.

        Args:
            session_id: Session identifier

        Returns:
            Resumed session DTO

        Raises:
            SessionNotFoundError: If session doesn't exist
            InvalidSessionStateError: If session cannot be resumed
        """
        session = await self._repository.get_session_by_id(session_id)
        if session is None:
            raise SessionNotFoundError(session_id)

        # Check if session can be resumed
        if session.status not in (SessionStatus.ACTIVE, SessionStatus.PAUSED):
            raise InvalidSessionStateError(
                session_id=session_id,
                current_status=session.status.value if hasattr(session.status, 'value') else str(session.status),
                operation="resume",
                allowed_statuses=["active", "paused"],
            )

        # Update session state
        now = datetime.now(timezone.utc)
        session.status = SessionStatus.ACTIVE
        session.last_activity_at = now
        session.updated_at = now

        updated = await self._repository.update_session(session)
        return self._model_to_dto(updated)

    async def pause_session(self, session_id: int) -> SessionDTO:
        """Pause an active session.

        Args:
            session_id: Session identifier

        Returns:
            Paused session DTO

        Raises:
            SessionNotFoundError: If session doesn't exist
            InvalidSessionStateError: If session is not active
        """
        session = await self._repository.get_session_by_id(session_id)
        if session is None:
            raise SessionNotFoundError(session_id)

        if session.status != SessionStatus.ACTIVE:
            raise InvalidSessionStateError(
                session_id=session_id,
                current_status=session.status.value if hasattr(session.status, 'value') else str(session.status),
                operation="pause",
                allowed_statuses=["active"],
            )

        now = datetime.now(timezone.utc)
        session.status = SessionStatus.PAUSED
        session.last_activity_at = now
        session.updated_at = now

        updated = await self._repository.update_session(session)
        return self._model_to_dto(updated)

    async def complete_session(
        self,
        session_id: int,
        force: bool = False,
    ) -> SessionDTO:
        """Complete a review session.

        Args:
            session_id: Session identifier
            force: If True, complete even with pending items

        Returns:
            Completed session DTO

        Raises:
            SessionNotFoundError: If session doesn't exist
            InvalidSessionStateError: If session cannot be completed
            PendingItemsError: If session has pending items and force=False
        """
        session = await self._repository.get_session_by_id(session_id)
        if session is None:
            raise SessionNotFoundError(session_id)

        if session.status in (SessionStatus.COMPLETED, SessionStatus.ABANDONED):
            raise InvalidSessionStateError(
                session_id=session_id,
                current_status=session.status.value if hasattr(session.status, 'value') else str(session.status),
                operation="complete",
                allowed_statuses=["active", "paused"],
            )

        # Check for pending items
        pending_count = session.total_items - session.items_reviewed
        if pending_count > 0 and not force:
            raise PendingItemsError(session_id=session_id, pending_count=pending_count)

        now = datetime.now(timezone.utc)
        session.status = SessionStatus.COMPLETED
        session.completed_at = now
        session.last_activity_at = now
        session.updated_at = now

        updated = await self._repository.update_session(session)
        return self._model_to_dto(updated)

    async def abandon_session(self, session_id: int) -> SessionDTO:
        """Abandon a session.

        Used when user starts a new session, abandoning the previous one.

        Args:
            session_id: Session identifier

        Returns:
            Abandoned session DTO

        Raises:
            SessionNotFoundError: If session doesn't exist
            InvalidSessionStateError: If session cannot be abandoned
        """
        session = await self._repository.get_session_by_id(session_id)
        if session is None:
            raise SessionNotFoundError(session_id)

        if session.status in (SessionStatus.COMPLETED, SessionStatus.ABANDONED):
            raise InvalidSessionStateError(
                session_id=session_id,
                current_status=session.status.value if hasattr(session.status, 'value') else str(session.status),
                operation="abandon",
                allowed_statuses=["active", "paused"],
            )

        now = datetime.now(timezone.utc)
        session.status = SessionStatus.ABANDONED
        session.last_activity_at = now
        session.updated_at = now

        updated = await self._repository.update_session(session)
        return self._model_to_dto(updated)

    async def update_progress(
        self,
        session_id: int,
        position: int,
        decision_type: str,
    ) -> SessionDTO:
        """Update session progress after a decision.

        Increments the appropriate counter based on decision type and
        updates the current position.

        Args:
            session_id: Session identifier
            position: New queue position
            decision_type: Type of decision (accept, reject, modify, skip)

        Returns:
            Updated session DTO

        Raises:
            SessionNotFoundError: If session doesn't exist
            InvalidSessionStateError: If session is not active
        """
        session = await self._repository.get_session_by_id(session_id)
        if session is None:
            raise SessionNotFoundError(session_id)

        if session.status != SessionStatus.ACTIVE:
            raise InvalidSessionStateError(
                session_id=session_id,
                current_status=session.status.value if hasattr(session.status, 'value') else str(session.status),
                operation="update_progress",
                allowed_statuses=["active"],
            )

        # Update counters
        session.items_reviewed += 1
        session.current_position = position

        if decision_type == "accept":
            session.items_accepted += 1
        elif decision_type == "reject":
            session.items_rejected += 1
        elif decision_type == "modify":
            session.items_modified += 1
        elif decision_type == "skip":
            session.items_skipped += 1

        now = datetime.now(timezone.utc)
        session.last_activity_at = now
        session.updated_at = now

        updated = await self._repository.update_session(session)
        return self._model_to_dto(updated)

    def _model_to_dto(self, model: Any) -> SessionDTO:
        """Convert database model to DTO.

        Handles both dictionary-like and object-like models from the
        repository layer.

        Args:
            model: Database model or dictionary

        Returns:
            SessionDTO instance
        """
        if isinstance(model, dict):
            return SessionDTO(**model)

        # Handle SQLAlchemy model or similar object
        return SessionDTO(
            id=model.id,
            session_uuid=model.session_uuid,
            tenant_id=model.tenant_id,
            user_email=model.user_email,
            status=model.status,
            started_at=model.started_at,
            last_activity_at=model.last_activity_at,
            completed_at=model.completed_at,
            expires_at=model.expires_at,
            current_position=model.current_position,
            total_items=model.total_items,
            items_reviewed=model.items_reviewed,
            items_accepted=model.items_accepted,
            items_rejected=model.items_rejected,
            items_modified=model.items_modified,
            items_skipped=model.items_skipped,
            review_mode=model.review_mode,
            priority_mode=model.priority_mode,
            filter_criteria=model.filter_criteria,
            session_metadata=model.session_metadata,
            created_at=model.created_at,
            updated_at=model.updated_at,
        )
