"""Custom exceptions for the Daily Review workflow.

Provides structured error handling with error codes matching the CLI API contract
defined in specs/006-daily-review/contracts/cli-api.md.
"""

from __future__ import annotations

from typing import Any


class ReviewError(Exception):
    """Base exception for review workflow errors.

    Attributes:
        code: Error code from contract (e.g., REVIEW_001)
        message: Human-readable error message
        details: Additional context for debugging
        recoverable: Whether the user can retry the operation
    """

    def __init__(
        self,
        code: str,
        message: str,
        details: dict[str, Any] | None = None,
        recoverable: bool = True,
    ) -> None:
        """Initialize ReviewError.

        Args:
            code: Error code (e.g., REVIEW_001)
            message: Human-readable error message
            details: Additional error context
            recoverable: Whether the error is recoverable
        """
        self.code = code
        self.message = message
        self.details = details or {}
        self.recoverable = recoverable
        super().__init__(message)

    def __repr__(self) -> str:
        """Return string representation of the error."""
        return f"{self.__class__.__name__}(code={self.code!r}, message={self.message!r})"


class SessionNotFoundError(ReviewError):
    """Session not found (REVIEW_001)."""

    def __init__(self, session_id: int) -> None:
        """Initialize SessionNotFoundError.

        Args:
            session_id: The session ID that was not found
        """
        super().__init__(
            code="REVIEW_001",
            message=f"Session {session_id} not found",
            details={"session_id": session_id},
            recoverable=False,
        )


class ActiveSessionExistsError(ReviewError):
    """Active session already exists for user (REVIEW_002)."""

    def __init__(self, existing_session_id: int, user_email: str) -> None:
        """Initialize ActiveSessionExistsError.

        Args:
            existing_session_id: ID of the existing active session
            user_email: User who has the active session
        """
        super().__init__(
            code="REVIEW_002",
            message=f"Active session already exists for {user_email}",
            details={
                "existing_session_id": existing_session_id,
                "user_email": user_email,
            },
            recoverable=True,
        )


class SessionExpiredError(ReviewError):
    """Session has expired (REVIEW_003)."""

    def __init__(self, session_id: int) -> None:
        """Initialize SessionExpiredError.

        Args:
            session_id: The expired session ID
        """
        super().__init__(
            code="REVIEW_003",
            message=f"Session {session_id} has expired",
            details={"session_id": session_id},
            recoverable=False,
        )


class InvalidSessionStateError(ReviewError):
    """Invalid session state for requested operation (REVIEW_003)."""

    def __init__(
        self,
        session_id: int,
        current_status: str,
        operation: str,
        allowed_statuses: list[str] | None = None,
    ) -> None:
        """Initialize InvalidSessionStateError.

        Args:
            session_id: The session ID
            current_status: Current status of the session
            operation: Operation that was attempted
            allowed_statuses: List of statuses that would allow the operation
        """
        super().__init__(
            code="REVIEW_003",
            message=f"Cannot {operation} session {session_id} in {current_status} state",
            details={
                "session_id": session_id,
                "current_status": current_status,
                "operation": operation,
                "allowed_statuses": allowed_statuses or [],
            },
            recoverable=False,
        )


class ItemNotFoundError(ReviewError):
    """Item not found in queue (REVIEW_004)."""

    def __init__(self, item_id: int, session_id: int | None = None) -> None:
        """Initialize ItemNotFoundError.

        Args:
            item_id: The item ID that was not found
            session_id: The session context (if any)
        """
        details: dict[str, Any] = {"item_id": item_id}
        if session_id is not None:
            details["session_id"] = session_id

        super().__init__(
            code="REVIEW_004",
            message=f"Item {item_id} not found in queue",
            details=details,
            recoverable=False,
        )


class UndoNotEligibleError(ReviewError):
    """Undo operation not eligible (REVIEW_005)."""

    def __init__(self, item_id: int, reason: str) -> None:
        """Initialize UndoNotEligibleError.

        Args:
            item_id: The item ID that cannot be undone
            reason: Reason why undo is not available
        """
        super().__init__(
            code="REVIEW_005",
            message=f"Cannot undo decision for item {item_id}: {reason}",
            details={"item_id": item_id, "reason": reason},
            recoverable=False,
        )


class PendingItemsError(ReviewError):
    """Operation failed due to pending items (REVIEW_006)."""

    def __init__(self, session_id: int, pending_count: int) -> None:
        """Initialize PendingItemsError.

        Args:
            session_id: The session ID
            pending_count: Number of pending items
        """
        super().__init__(
            code="REVIEW_006",
            message=f"Session {session_id} has {pending_count} pending items",
            details={
                "session_id": session_id,
                "pending_items": pending_count,
            },
            recoverable=True,
        )


class BatchOperationError(ReviewError):
    """Batch operation failed (REVIEW_006)."""

    def __init__(self, batch_id: str, reason: str) -> None:
        """Initialize BatchOperationError.

        Args:
            batch_id: The batch operation ID
            reason: Reason for failure
        """
        super().__init__(
            code="REVIEW_006",
            message=f"Batch operation {batch_id} failed: {reason}",
            details={"batch_id": batch_id, "reason": reason},
            recoverable=True,
        )


class InvalidFilterCriteriaError(ReviewError):
    """Invalid filter criteria (REVIEW_007)."""

    def __init__(self, criteria: dict[str, Any], reason: str) -> None:
        """Initialize InvalidFilterCriteriaError.

        Args:
            criteria: The invalid filter criteria
            reason: Why the criteria are invalid
        """
        super().__init__(
            code="REVIEW_007",
            message=f"Invalid filter criteria: {reason}",
            details={"criteria": criteria, "reason": reason},
            recoverable=True,
        )


class DatabaseOperationError(ReviewError):
    """Database operation failed (REVIEW_008)."""

    def __init__(self, operation: str, reason: str) -> None:
        """Initialize DatabaseOperationError.

        Args:
            operation: The database operation that failed
            reason: Reason for failure
        """
        super().__init__(
            code="REVIEW_008",
            message=f"Database operation '{operation}' failed: {reason}",
            details={"operation": operation, "reason": reason},
            recoverable=True,
        )
