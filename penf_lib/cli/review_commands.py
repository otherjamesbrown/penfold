"""Click command group for the daily review workflow.

This module implements the `penf review` command group for User Stories 1-4:
- User Story 1: Queue Management (start, resume, status, queue, next)
- User Story 2: Validation and Correction (accept, reject, modify, skip, undo)
- User Story 3: Batch Operations and Completion (batch, complete)
- User Story 4: Learning Rules Management (rules subcommand group)

Commands:
    - penf review: Start or resume a review session
    - penf review status: Show session status
    - penf review queue: Display the review queue
    - penf review next: Show next item in queue
    - penf review accept: Accept AI suggestion
    - penf review reject: Reject AI suggestion
    - penf review modify: Modify AI suggestion
    - penf review skip: Skip item for later
    - penf review undo: Undo recent decision
    - penf review batch: Apply batch operations to similar items
    - penf review complete: Complete the current review session
    - penf review rules: Manage learning rules (subcommand group)
        - penf review rules list: List learning rules
        - penf review rules show: Show rule details
        - penf review rules enable: Enable a disabled rule
        - penf review rules disable: Disable an active rule
        - penf review rules pending: View pending rule suggestions
        - penf review rules accept: Accept a pending suggestion
        - penf review rules reject: Reject a pending suggestion
"""

from __future__ import annotations

import asyncio
import sys
from datetime import datetime, timedelta, timezone
from decimal import Decimal
from typing import Any
from uuid import UUID, uuid4

import click
from rich.console import Console
from rich.progress import Progress, SpinnerColumn, TextColumn

from penf_lib.cli.review_display import (
    render_item,
    render_queue,
    render_session_status,
)
from penf_lib.review.exceptions import (
    ActiveSessionExistsError,
    ReviewError,
)
from penf_lib.review.models import (
    AISuggestion,
    ContentType,
    DecisionType,
    LearningRuleDTO,
    LearningRuleStatus,
    LearningRuleType,
    PriorityMode,
    ReviewItemDTO,
    ReviewItemStatus,
    ReviewMode,
    SessionDTO,
    SessionStatus,
    UserCorrection,
    UserFeedbackDTO,
)
from penf_lib.review.queue import QueueManager
from penf_lib.review.session import SessionManager

# Lazy import to avoid loading redis at module load time
# from penf_lib.storage.connections import cleanup_connections

# Initialize rich console
console = Console()


# =============================================================================
# MOCK REPOSITORY FOR DEVELOPMENT
# =============================================================================


class MockReviewRepository:
    """Mock repository for development and testing.

    Provides in-memory storage for review sessions and items.
    Will be replaced by actual database repository in production.
    """

    def __init__(self) -> None:
        """Initialize mock repository with sample data."""
        self._sessions: dict[int, SessionDTO] = {}
        self._items: dict[int, ReviewItemDTO] = {}
        self._session_counter = 0
        self._item_counter = 0
        self._active_sessions: dict[tuple[UUID, str], int] = {}

    async def create_session(self, session_data: dict[str, Any]) -> SessionDTO:
        """Create a new session in mock storage."""
        self._session_counter += 1
        session = SessionDTO(
            id=self._session_counter,
            **session_data,
        )
        self._sessions[session.id] = session
        self._active_sessions[(session.tenant_id, session.user_email)] = session.id
        return session

    async def get_session_by_id(self, session_id: int) -> SessionDTO | None:
        """Get session by ID from mock storage."""
        return self._sessions.get(session_id)

    async def get_active_session_for_user(
        self,
        tenant_id: UUID,
        user_email: str,
    ) -> SessionDTO | None:
        """Get active session for a user from mock storage."""
        session_id = self._active_sessions.get((tenant_id, user_email))
        if session_id is None:
            return None
        session = self._sessions.get(session_id)
        if session and session.status in (SessionStatus.ACTIVE, SessionStatus.PAUSED):
            return session
        return None

    async def update_session(self, session: SessionDTO) -> SessionDTO:
        """Update session in mock storage."""
        self._sessions[session.id] = session
        # Clean up active session tracking if session is no longer active
        if session.status in (SessionStatus.COMPLETED, SessionStatus.ABANDONED):
            key = (session.tenant_id, session.user_email)
            if key in self._active_sessions:
                del self._active_sessions[key]
        return session

    async def get_pending_items_for_queue(
        self,
        tenant_id: UUID,
        limit: int,
        confidence_threshold: float,
    ) -> list[ReviewItemDTO]:
        """Get pending items for queue from mock storage."""
        # Generate sample items for demonstration
        return self._generate_sample_items(tenant_id, limit)

    async def get_queue_items(
        self,
        session_id: int,
        status: list[str] | None,
        limit: int,
        offset: int,
    ) -> list[ReviewItemDTO]:
        """Get items in queue from mock storage."""
        session = self._sessions.get(session_id)
        if not session:
            return []

        # Filter items by session and status
        items = [
            item
            for item in self._items.values()
            if item.session_id == session_id
            and (status is None or item.status.value in status)
        ]

        # Sort by queue position
        items.sort(key=lambda x: x.queue_position or 0)

        # Apply pagination
        return items[offset : offset + limit]

    async def update_item_queue_position(
        self,
        item_id: int,
        session_id: int,
        queue_position: int,
    ) -> ReviewItemDTO:
        """Update an item's queue position in mock storage."""
        item = self._items.get(item_id)
        if item:
            updated = item.model_copy(
                update={
                    "session_id": session_id,
                    "queue_position": queue_position,
                }
            )
            self._items[item_id] = updated
            return updated
        raise ValueError(f"Item {item_id} not found")

    def _generate_sample_items(
        self,
        tenant_id: UUID,
        limit: int,
    ) -> list[ReviewItemDTO]:
        """Generate sample review items for demonstration."""
        now = datetime.now(timezone.utc)
        sample_data = [
            {
                "content_type": ContentType.EMAIL,
                "content_preview": "Re: Q1 Planning Meeting - Thanks for the update...",
                "ai_suggestion": AISuggestion(
                    category="project/planning",
                    participants=["John Doe", "Jane Smith"],
                    tags=["planning", "q1", "meeting"],
                ),
                "ai_confidence": Decimal("0.45"),
                "ai_model": "gemini-pro",
                "business_importance": 7,
                "source_timestamp": now - timedelta(hours=2),
            },
            {
                "content_type": ContentType.EMAIL,
                "content_preview": "Project update: Alpha release - We are on track for...",
                "ai_suggestion": AISuggestion(
                    category="project/releases",
                    participants=["Dev Team"],
                    tags=["release", "alpha", "update"],
                ),
                "ai_confidence": Decimal("0.52"),
                "ai_model": "gemini-pro",
                "business_importance": 8,
                "source_timestamp": now - timedelta(hours=5),
            },
            {
                "content_type": ContentType.MEETING,
                "content_preview": "Weekly Standup 2026-01-14 - Discussion of blockers...",
                "ai_suggestion": AISuggestion(
                    category="meetings/standup",
                    participants=["Engineering Team"],
                    tags=["standup", "weekly", "team"],
                ),
                "ai_confidence": Decimal("0.61"),
                "ai_model": "gemini-pro",
                "business_importance": 5,
                "source_timestamp": now - timedelta(days=1),
            },
            {
                "content_type": ContentType.EMAIL,
                "content_preview": "Budget Review Q1 - Please review the attached...",
                "ai_suggestion": AISuggestion(
                    category="finance/budget",
                    participants=["Finance Team", "CFO"],
                    tags=["budget", "finance", "review"],
                ),
                "ai_confidence": Decimal("0.73"),
                "ai_model": "gemini-pro",
                "business_importance": 9,
                "source_timestamp": now - timedelta(hours=8),
            },
            {
                "content_type": ContentType.DOCUMENT,
                "content_preview": "Technical Specification v2.0 - Architecture overview...",
                "ai_suggestion": AISuggestion(
                    category="documentation/technical",
                    participants=["Architecture Team"],
                    tags=["architecture", "spec", "technical"],
                ),
                "ai_confidence": Decimal("0.38"),
                "ai_model": "gemini-pro",
                "business_importance": 6,
                "source_timestamp": now - timedelta(days=2),
            },
        ]

        items: list[ReviewItemDTO] = []
        for i, data in enumerate(sample_data[:limit], start=1):
            self._item_counter += 1
            item = ReviewItemDTO(
                id=self._item_counter,
                tenant_id=tenant_id,
                session_id=None,
                source_id=i,
                processing_result_id=uuid4(),
                queue_position=None,
                status=ReviewItemStatus.PENDING,
                created_at=now,
                updated_at=now,
                **data,
            )
            self._items[item.id] = item
            items.append(item)

        return items


# Global mock repository instance
_mock_repository: MockReviewRepository | None = None


def get_mock_repository() -> MockReviewRepository:
    """Get or create the mock repository singleton."""
    global _mock_repository
    if _mock_repository is None:
        _mock_repository = MockReviewRepository()
    return _mock_repository


class MockSessionState:
    """Tracks the current item and decision history for a session.

    Provides state management for the validation/correction commands,
    tracking which item is current and enabling undo functionality.
    """

    def __init__(self) -> None:
        """Initialize mock session state."""
        self._current_item_id: int | None = None
        self._decision_history: list[dict[str, Any]] = []
        self._batch_counter = 0

    @property
    def current_item_id(self) -> int | None:
        """Get the current item ID."""
        return self._current_item_id

    @current_item_id.setter
    def current_item_id(self, item_id: int | None) -> None:
        """Set the current item ID."""
        self._current_item_id = item_id

    def record_decision(
        self,
        item: ReviewItemDTO,
        decision: DecisionType,
        batch_id: UUID | None = None,
        correction: UserCorrection | None = None,
    ) -> None:
        """Record a decision for undo support.

        Args:
            item: The item that was decided on
            decision: The decision type (accept, reject, modify, skip)
            batch_id: Optional batch operation ID
            correction: Optional user correction for modify decisions
        """
        self._decision_history.append({
            "item_id": item.id,
            "item": item,
            "decision": decision,
            "batch_id": batch_id,
            "correction": correction,
            "timestamp": datetime.now(timezone.utc),
            "original_status": item.status,
            "original_decision": item.user_decision,
            "original_correction": item.user_correction,
        })

    def get_recent_decisions(self, count: int = 1) -> list[dict[str, Any]]:
        """Get the most recent decisions for undo.

        Args:
            count: Number of recent decisions to retrieve

        Returns:
            List of decision records, most recent first
        """
        return list(reversed(self._decision_history[-count:]))

    def pop_decisions(self, count: int = 1) -> list[dict[str, Any]]:
        """Remove and return the most recent decisions.

        Args:
            count: Number of decisions to pop

        Returns:
            List of popped decision records
        """
        popped = []
        for _ in range(min(count, len(self._decision_history))):
            popped.append(self._decision_history.pop())
        return popped

    def get_decisions_by_batch(self, batch_id: UUID) -> list[dict[str, Any]]:
        """Get all decisions for a specific batch.

        Args:
            batch_id: The batch ID to filter by

        Returns:
            List of decision records for the batch
        """
        return [d for d in self._decision_history if d.get("batch_id") == batch_id]

    def generate_batch_id(self) -> UUID:
        """Generate a new batch ID."""
        return uuid4()


class MockFeedbackManager:
    """Mock feedback manager for development.

    Provides in-memory storage for user feedback records during
    the validation/correction workflow.
    """

    def __init__(self) -> None:
        """Initialize mock feedback manager."""
        self._feedback: list[UserFeedbackDTO] = []
        self._counter = 0

    async def record_accept(
        self,
        item: ReviewItemDTO,
        session_id: int,
        time_spent_ms: int = 0,
        batch_id: UUID | None = None,
    ) -> UserFeedbackDTO:
        """Record an accept decision.

        Args:
            item: The item that was accepted
            session_id: The session ID
            time_spent_ms: Time spent on decision in milliseconds
            batch_id: Optional batch operation ID

        Returns:
            The created feedback record
        """
        self._counter += 1
        feedback = UserFeedbackDTO(
            id=self._counter,
            feedback_uuid=uuid4(),
            tenant_id=item.tenant_id,
            review_item_id=item.id,
            session_id=session_id,
            decision_type=DecisionType.ACCEPT,
            original_suggestion=item.ai_suggestion,
            user_correction=None,
            time_spent_ms=time_spent_ms,
            was_batch_decision=batch_id is not None,
            batch_id=batch_id,
            created_at=datetime.now(timezone.utc),
        )
        self._feedback.append(feedback)
        return feedback

    async def record_reject(
        self,
        item: ReviewItemDTO,
        session_id: int,
        reason: str | None = None,
        time_spent_ms: int = 0,
        batch_id: UUID | None = None,
    ) -> UserFeedbackDTO:
        """Record a reject decision.

        Args:
            item: The item that was rejected
            session_id: The session ID
            reason: Optional rejection reason code
            time_spent_ms: Time spent on decision
            batch_id: Optional batch operation ID

        Returns:
            The created feedback record
        """
        self._counter += 1
        correction = UserCorrection(reason=reason) if reason else None
        feedback = UserFeedbackDTO(
            id=self._counter,
            feedback_uuid=uuid4(),
            tenant_id=item.tenant_id,
            review_item_id=item.id,
            session_id=session_id,
            decision_type=DecisionType.REJECT,
            original_suggestion=item.ai_suggestion,
            user_correction=correction,
            correction_reason=reason,
            time_spent_ms=time_spent_ms,
            was_batch_decision=batch_id is not None,
            batch_id=batch_id,
            created_at=datetime.now(timezone.utc),
        )
        self._feedback.append(feedback)
        return feedback

    async def record_modify(
        self,
        item: ReviewItemDTO,
        session_id: int,
        correction: UserCorrection,
        time_spent_ms: int = 0,
        batch_id: UUID | None = None,
    ) -> UserFeedbackDTO:
        """Record a modify decision.

        Args:
            item: The item that was modified
            session_id: The session ID
            correction: The user's correction
            time_spent_ms: Time spent on decision
            batch_id: Optional batch operation ID

        Returns:
            The created feedback record
        """
        self._counter += 1
        feedback = UserFeedbackDTO(
            id=self._counter,
            feedback_uuid=uuid4(),
            tenant_id=item.tenant_id,
            review_item_id=item.id,
            session_id=session_id,
            decision_type=DecisionType.MODIFY,
            original_suggestion=item.ai_suggestion,
            user_correction=correction,
            correction_reason=correction.reason,
            correction_notes=correction.notes,
            time_spent_ms=time_spent_ms,
            was_batch_decision=batch_id is not None,
            batch_id=batch_id,
            created_at=datetime.now(timezone.utc),
        )
        self._feedback.append(feedback)
        return feedback

    async def record_skip(
        self,
        item: ReviewItemDTO,
        session_id: int,
        reason: str | None = None,
        time_spent_ms: int = 0,
    ) -> UserFeedbackDTO:
        """Record a skip decision.

        Args:
            item: The item that was skipped
            session_id: The session ID
            reason: Optional skip reason
            time_spent_ms: Time spent on decision

        Returns:
            The created feedback record
        """
        self._counter += 1
        correction = UserCorrection(notes=reason) if reason else None
        feedback = UserFeedbackDTO(
            id=self._counter,
            feedback_uuid=uuid4(),
            tenant_id=item.tenant_id,
            review_item_id=item.id,
            session_id=session_id,
            decision_type=DecisionType.SKIP,
            original_suggestion=item.ai_suggestion,
            user_correction=correction,
            correction_notes=reason,
            time_spent_ms=time_spent_ms,
            was_batch_decision=False,
            batch_id=None,
            created_at=datetime.now(timezone.utc),
        )
        self._feedback.append(feedback)
        return feedback


# Global mock instances
_mock_session_state: MockSessionState | None = None
_mock_feedback_manager: MockFeedbackManager | None = None


def get_mock_session_state() -> MockSessionState:
    """Get or create the mock session state singleton."""
    global _mock_session_state
    if _mock_session_state is None:
        _mock_session_state = MockSessionState()
    return _mock_session_state


def get_mock_feedback_manager() -> MockFeedbackManager:
    """Get or create the mock feedback manager singleton."""
    global _mock_feedback_manager
    if _mock_feedback_manager is None:
        _mock_feedback_manager = MockFeedbackManager()
    return _mock_feedback_manager


# =============================================================================
# ERROR DISPLAY HELPER
# =============================================================================


def display_error(error: ReviewError) -> None:
    """Display a review error with formatting.

    Args:
        error: The ReviewError to display
    """
    console.print(f"\n[red]Error ({error.code}):[/red] {error.message}")
    if error.details:
        for key, value in error.details.items():
            console.print(f"  [dim]{key}:[/dim] {value}")
    if error.recoverable:
        console.print("\n[dim]This error may be recoverable. Try again or use --help for options.[/dim]")


def estimate_time_remaining(session: SessionDTO, seconds_per_item: int = 10) -> int | None:
    """Estimate remaining review time in minutes.

    Args:
        session: The session to estimate for
        seconds_per_item: Average seconds per item (default 10)

    Returns:
        Estimated minutes remaining, or None if complete
    """
    remaining_items = session.total_items - session.items_reviewed
    if remaining_items <= 0:
        return None
    return int((remaining_items * seconds_per_item) / 60)


# =============================================================================
# CLICK COMMAND GROUP
# =============================================================================


@click.group(name="review", invoke_without_command=True)
@click.option(
    "--mode",
    type=click.Choice(["quick", "standard", "detailed"]),
    default="standard",
    help="Review mode affecting detail level",
)
@click.option(
    "--priority",
    type=click.Choice(["confidence", "importance", "recency", "mixed"]),
    default="mixed",
    help="Queue prioritization strategy",
)
@click.option(
    "--filter",
    "filter_expr",
    type=str,
    default=None,
    help="Filter expression for queue items",
)
@click.option(
    "--limit",
    type=int,
    default=100,
    help="Maximum items to queue",
)
@click.option(
    "--resume/--no-resume",
    default=True,
    help="Resume active session if exists (default: resume)",
)
@click.option(
    "--new",
    "force_new",
    is_flag=True,
    default=False,
    help="Force new session (abandon existing)",
)
@click.pass_context
def review_group(
    ctx: click.Context,
    mode: str,
    priority: str,
    filter_expr: str | None,
    limit: int,
    resume: bool,
    force_new: bool,
) -> None:
    """Daily review workflow commands.

    Start or resume a review session to validate AI suggestions
    for your content. Use subcommands for specific operations.

    Examples:
        penf review                    # Start/resume standard review
        penf review --mode quick       # Quick review mode
        penf review --new              # Force new session
        penf review status             # Show session status
        penf review queue              # Display queue
        penf review next               # Show next item
    """
    ctx.ensure_object(dict)

    # Store options in context for subcommands
    ctx.obj["review_mode"] = ReviewMode(mode)
    ctx.obj["priority_mode"] = PriorityMode(priority)
    ctx.obj["filter_expr"] = filter_expr
    ctx.obj["limit"] = limit
    ctx.obj["resume"] = resume
    ctx.obj["force_new"] = force_new

    # If no subcommand invoked, run the main review command
    if ctx.invoked_subcommand is None:
        _run_review_main(ctx)


def _run_review_main(ctx: click.Context) -> None:
    """Run the main review command logic."""
    async def _review_async() -> int:
        try:
            # Get tenant context from CLI context
            tenant_id_str = ctx.obj.get("tenant_id")
            user_email = ctx.obj.get("user_email")

            # Use defaults if not set (for development)
            if not tenant_id_str:
                tenant_id = UUID("00000000-0000-0000-0000-000000000001")
            else:
                tenant_id = UUID(tenant_id_str)

            if not user_email:
                user_email = "developer@example.com"

            # Get repository and managers
            repository = get_mock_repository()
            session_manager = SessionManager(repository)
            queue_manager = QueueManager(repository)

            # Check for existing session
            with Progress(
                SpinnerColumn(),
                TextColumn("[progress.description]{task.description}"),
                console=console,
                transient=True,
            ) as progress:
                progress.add_task("Checking for active session...", total=None)
                existing_session = await session_manager.get_active_session(
                    tenant_id, user_email
                )

            review_mode = ctx.obj["review_mode"]
            priority_mode = ctx.obj["priority_mode"]
            filter_expr = ctx.obj["filter_expr"]
            limit = ctx.obj["limit"]
            force_new = ctx.obj["force_new"]
            should_resume = ctx.obj["resume"]

            session: SessionDTO

            if existing_session and force_new:
                # Abandon existing session and create new one
                console.print("[yellow]Abandoning existing session...[/yellow]")
                await session_manager.abandon_session(existing_session.id)
                existing_session = None

            if existing_session and should_resume:
                # Resume existing session
                console.print("[green]Resuming existing session...[/green]")
                session = await session_manager.resume_session(existing_session.id)
                est_time = estimate_time_remaining(session)
                render_session_status(console, session, est_time)

            else:
                # Create new session
                console.print("[blue]Creating new review session...[/blue]")

                # Parse filter criteria if provided
                filter_criteria: dict[str, Any] | None = None
                if filter_expr:
                    # Simple filter parsing (key:value pairs)
                    filter_criteria = {}
                    for part in filter_expr.split(","):
                        if ":" in part:
                            key, value = part.strip().split(":", 1)
                            filter_criteria[key.strip()] = value.strip()

                # Populate queue first to get item count
                with Progress(
                    SpinnerColumn(),
                    TextColumn("[progress.description]{task.description}"),
                    console=console,
                    transient=True,
                ) as progress:
                    progress.add_task("Populating review queue...", total=None)
                    queue_items = await queue_manager.populate_queue(
                        tenant_id=tenant_id,
                        session_id=0,  # Temporary, will update after session creation
                        priority_mode=priority_mode,
                        filter_criteria=filter_criteria,
                        limit=limit,
                    )

                if not queue_items:
                    console.print("[green]No items require review.[/green]")
                    console.print("[dim]All AI suggestions meet the confidence threshold.[/dim]")
                    return 0

                # Create session with item count
                session = await session_manager.create_session(
                    tenant_id=tenant_id,
                    user_email=user_email,
                    total_items=len(queue_items),
                    mode=review_mode,
                    priority=priority_mode,
                    filter_criteria=filter_criteria,
                )

                # Store items in repository with session ID
                for item in queue_items:
                    updated_item = item.model_copy(update={"session_id": session.id})
                    repository._items[item.id] = updated_item

                est_time = estimate_time_remaining(session)
                render_session_status(console, session, est_time)

                # Show first item
                console.print("\n")
                if queue_items:
                    render_item(console, queue_items[0], 1, len(queue_items))

            return 0

        except ActiveSessionExistsError as e:
            console.print("\n[yellow]Active session already exists.[/yellow]")
            console.print(f"Session ID: {e.details['existing_session_id']}")
            console.print("\n[dim]Use --new to start a fresh session, or run without --no-resume to continue.[/dim]")
            return 1

        except ReviewError as e:
            display_error(e)
            return 1

        except Exception as e:
            console.print(f"\n[red]Unexpected error:[/red] {e}")
            return 1

        finally:
            from penf_lib.storage.connections import cleanup_connections
            await cleanup_connections()

    result = asyncio.run(_review_async())
    sys.exit(result)


@review_group.command("status")
@click.pass_context
def review_status(ctx: click.Context) -> None:
    """Show current review session status.

    Displays progress, statistics, and estimated time remaining
    for the active review session.
    """
    async def _status_async() -> int:
        try:
            # Get tenant context
            tenant_id_str = ctx.obj.get("tenant_id")
            user_email = ctx.obj.get("user_email")

            if not tenant_id_str:
                tenant_id = UUID("00000000-0000-0000-0000-000000000001")
            else:
                tenant_id = UUID(tenant_id_str)

            if not user_email:
                user_email = "developer@example.com"

            repository = get_mock_repository()
            session_manager = SessionManager(repository)

            session = await session_manager.get_active_session(tenant_id, user_email)

            if not session:
                console.print("[yellow]No active review session.[/yellow]")
                console.print("\n[dim]Start a review with: penf review[/dim]")
                return 0

            est_time = estimate_time_remaining(session)
            render_session_status(console, session, est_time)
            return 0

        except ReviewError as e:
            display_error(e)
            return 1

        except Exception as e:
            console.print(f"\n[red]Unexpected error:[/red] {e}")
            return 1

        finally:
            from penf_lib.storage.connections import cleanup_connections
            await cleanup_connections()

    result = asyncio.run(_status_async())
    sys.exit(result)


@review_group.command("queue")
@click.option(
    "--all",
    "show_all",
    is_flag=True,
    default=False,
    help="Show all items (not just pending)",
)
@click.option(
    "--filter",
    "filter_expr",
    type=str,
    default=None,
    help="Filter expression",
)
@click.option(
    "--limit",
    type=int,
    default=20,
    help="Items per page",
)
@click.option(
    "--page",
    type=int,
    default=1,
    help="Page number",
)
@click.pass_context
def review_queue(
    ctx: click.Context,
    show_all: bool,
    filter_expr: str | None,
    limit: int,
    page: int,
) -> None:
    """Display the review queue.

    Shows pending items in the current review session with their
    AI confidence scores and suggested categories.

    Examples:
        penf review queue              # Show pending items
        penf review queue --all        # Show all items
        penf review queue --limit 50   # Show 50 items per page
    """
    async def _queue_async() -> int:
        try:
            # Get tenant context
            tenant_id_str = ctx.obj.get("tenant_id")
            user_email = ctx.obj.get("user_email")

            if not tenant_id_str:
                tenant_id = UUID("00000000-0000-0000-0000-000000000001")
            else:
                tenant_id = UUID(tenant_id_str)

            if not user_email:
                user_email = "developer@example.com"

            repository = get_mock_repository()
            session_manager = SessionManager(repository)

            session = await session_manager.get_active_session(tenant_id, user_email)

            if not session:
                console.print("[yellow]No active review session.[/yellow]")
                console.print("\n[dim]Start a review with: penf review[/dim]")
                return 0

            # Determine which statuses to show
            if show_all:
                status_filter = None
            else:
                status_filter = [ReviewItemStatus.PENDING.value]

            # Calculate offset from page
            offset = (page - 1) * limit

            # Get queue items
            items = await repository.get_queue_items(
                session_id=session.id,
                status=status_filter,
                limit=limit,
                offset=offset,
            )

            if not items:
                if show_all:
                    console.print("[yellow]No items in queue.[/yellow]")
                else:
                    console.print("[green]No pending items in queue.[/green]")
                    console.print("[dim]All items have been reviewed.[/dim]")
                return 0

            # Calculate pagination info
            pending_count = session.total_items - session.items_reviewed
            total_count = session.total_items

            render_queue(
                console=console,
                items=items,
                pending_count=pending_count,
                total_count=total_count,
                items_per_page=limit,
                current_page=page,
            )

            return 0

        except ReviewError as e:
            display_error(e)
            return 1

        except Exception as e:
            console.print(f"\n[red]Unexpected error:[/red] {e}")
            return 1

        finally:
            from penf_lib.storage.connections import cleanup_connections
            await cleanup_connections()

    result = asyncio.run(_queue_async())
    sys.exit(result)


@review_group.command("next")
@click.option(
    "--skip",
    type=int,
    default=0,
    help="Skip N items",
)
@click.option(
    "--id",
    "item_id",
    type=int,
    default=None,
    help="Go to specific item ID",
)
@click.pass_context
def review_next(
    ctx: click.Context,
    skip: int,
    item_id: int | None,
) -> None:
    """Show next item in queue for review.

    Displays the next pending item with AI suggestions
    for user validation.

    Examples:
        penf review next           # Show next item
        penf review next --skip 3  # Skip 3 items
        penf review next --id 42   # Go to item 42
    """
    async def _next_async() -> int:
        try:
            # Get tenant context
            tenant_id_str = ctx.obj.get("tenant_id")
            user_email = ctx.obj.get("user_email")

            if not tenant_id_str:
                tenant_id = UUID("00000000-0000-0000-0000-000000000001")
            else:
                tenant_id = UUID(tenant_id_str)

            if not user_email:
                user_email = "developer@example.com"

            repository = get_mock_repository()
            session_manager = SessionManager(repository)
            queue_manager = QueueManager(repository)

            session = await session_manager.get_active_session(tenant_id, user_email)

            if not session:
                console.print("[yellow]No active review session.[/yellow]")
                console.print("\n[dim]Start a review with: penf review[/dim]")
                return 0

            if item_id is not None:
                # Go to specific item
                item = repository._items.get(item_id)
                if not item or item.session_id != session.id:
                    console.print(f"[red]Item {item_id} not found in current session.[/red]")
                    return 1
                render_item(
                    console,
                    item,
                    position=item.queue_position or 0,
                    total=session.total_items,
                )
            else:
                # Get next item from queue
                current_pos = session.current_position + skip
                item = await queue_manager.get_next_item(
                    session_id=session.id,
                    current_position=current_pos,
                )

                if not item:
                    console.print("[green]Queue complete![/green]")
                    console.print("[dim]All items have been reviewed.[/dim]")
                    console.print("\n[dim]Use 'penf review complete' to finish the session.[/dim]")
                    return 0

                render_item(
                    console,
                    item,
                    position=current_pos + 1,
                    total=session.total_items,
                )

            return 0

        except ReviewError as e:
            display_error(e)
            return 1

        except Exception as e:
            console.print(f"\n[red]Unexpected error:[/red] {e}")
            return 1

        finally:
            from penf_lib.storage.connections import cleanup_connections
            await cleanup_connections()

    result = asyncio.run(_next_async())
    sys.exit(result)


# =============================================================================
# VALIDATION AND CORRECTION COMMANDS (User Story 2)
# =============================================================================


async def _get_item_for_action(
    ctx: click.Context,
    item_id: int | None,
) -> tuple[SessionDTO | None, ReviewItemDTO | None, str | None]:
    """Get the item for a validation action.

    Resolves item from explicit ID or current session state.

    Args:
        ctx: Click context
        item_id: Optional explicit item ID

    Returns:
        Tuple of (session, item, error_message)
    """
    # Get tenant context
    tenant_id_str = ctx.obj.get("tenant_id")
    user_email = ctx.obj.get("user_email")

    if not tenant_id_str:
        tenant_id = UUID("00000000-0000-0000-0000-000000000001")
    else:
        tenant_id = UUID(tenant_id_str)

    if not user_email:
        user_email = "developer@example.com"

    repository = get_mock_repository()
    session_manager = SessionManager(repository)
    session_state = get_mock_session_state()

    session = await session_manager.get_active_session(tenant_id, user_email)

    if not session:
        return None, None, "No active review session. Start a review with: penf review"

    # Resolve item ID
    resolved_item_id = item_id
    if resolved_item_id is None:
        resolved_item_id = session_state.current_item_id

    if resolved_item_id is None:
        # Try to get the current item from session position
        queue_manager = QueueManager(repository)
        item = await queue_manager.get_next_item(
            session_id=session.id,
            current_position=session.current_position,
        )
        if item:
            resolved_item_id = item.id
            session_state.current_item_id = item.id

    if resolved_item_id is None:
        return session, None, "No current item. Use 'penf review next' to get an item."

    # Get the item
    item = repository._items.get(resolved_item_id)
    if not item or item.session_id != session.id:
        return session, None, f"Item {resolved_item_id} not found in current session."

    return session, item, None


async def _show_next_item(
    ctx: click.Context,
    session: SessionDTO,
) -> None:
    """Display the next item after an action.

    Args:
        ctx: Click context
        session: Current session
    """
    repository = get_mock_repository()
    queue_manager = QueueManager(repository)
    session_state = get_mock_session_state()

    # Get next pending item
    next_item = await queue_manager.get_next_item(
        session_id=session.id,
        current_position=session.current_position,
    )

    if next_item:
        session_state.current_item_id = next_item.id
        console.print()
        render_item(
            console,
            next_item,
            position=session.current_position + 1,
            total=session.total_items,
        )
    else:
        session_state.current_item_id = None
        console.print("\n[green]Queue complete![/green]")
        console.print("[dim]All items have been reviewed.[/dim]")
        console.print("\n[dim]Use 'penf review complete' to finish the session.[/dim]")


def _truncate_preview(text: str, max_length: int = 40) -> str:
    """Truncate text for preview display."""
    if len(text) <= max_length:
        return text
    return text[: max_length - 3] + "..."


@review_group.command("accept")
@click.argument("item_id", type=int, required=False)
@click.option(
    "--batch",
    type=click.Choice(["thread", "sender", "category"]),
    help="Batch mode: accept all items matching criteria",
)
@click.option(
    "--confirm",
    is_flag=True,
    help="Skip confirmation for batch operations",
)
@click.pass_context
def review_accept(
    ctx: click.Context,
    item_id: int | None,
    batch: str | None,
    confirm: bool,
) -> None:
    """Accept AI suggestion for current or specified item.

    Marks the AI suggestion as correct and moves to the next item.
    Use --batch to accept multiple related items at once.

    Examples:
        penf review accept           # Accept current item
        penf review accept 42        # Accept item 42
        penf review accept --batch sender  # Accept all from same sender
    """
    async def _accept_async() -> int:
        try:
            from rich.prompt import Confirm

            session, item, error = await _get_item_for_action(ctx, item_id)

            if error:
                console.print(f"[yellow]{error}[/yellow]")
                return 0 if session is None else 1

            assert session is not None
            assert item is not None

            repository = get_mock_repository()
            session_state = get_mock_session_state()
            feedback_manager = get_mock_feedback_manager()

            if batch:
                # Batch mode - find matching items
                matching_items: list[ReviewItemDTO] = []

                for repo_item in repository._items.values():
                    if repo_item.session_id != session.id:
                        continue
                    if repo_item.status != ReviewItemStatus.PENDING:
                        continue

                    if batch == "sender":
                        # Match by first participant (sender)
                        if (item.ai_suggestion.participants and
                            repo_item.ai_suggestion.participants and
                            item.ai_suggestion.participants[0] == repo_item.ai_suggestion.participants[0]):
                            matching_items.append(repo_item)
                    elif batch == "category":
                        # Match by category
                        if item.ai_suggestion.category == repo_item.ai_suggestion.category:
                            matching_items.append(repo_item)
                    elif batch == "thread":
                        # Match by source_id (thread grouping)
                        if item.source_id == repo_item.source_id:
                            matching_items.append(repo_item)

                if not matching_items:
                    console.print("[yellow]No matching items found for batch operation.[/yellow]")
                    return 0

                # Show preview
                console.print(f"\n[bold]Batch Accept - {len(matching_items)} items[/bold]")
                console.print(f"[dim]Matching by: {batch}[/dim]\n")

                for idx, match_item in enumerate(matching_items[:5], 1):
                    preview = _truncate_preview(match_item.content_preview)
                    console.print(f"  {idx}. [{match_item.content_type.value}] {preview}")

                if len(matching_items) > 5:
                    console.print(f"  ... and {len(matching_items) - 5} more")

                # Require confirmation
                if not confirm:
                    console.print()
                    if not Confirm.ask(f"Accept all {len(matching_items)} items?", default=False):
                        console.print("[dim]Batch operation cancelled.[/dim]")
                        return 0

                # Apply batch operation
                batch_id = session_state.generate_batch_id()
                for match_item in matching_items:
                    # Update item status
                    updated_item = match_item.model_copy(update={
                        "status": ReviewItemStatus.ACCEPTED,
                        "user_decision": DecisionType.ACCEPT,
                        "reviewed_at": datetime.now(timezone.utc),
                        "batch_id": batch_id,
                        "undo_eligible": True,
                        "undo_deadline": datetime.now(timezone.utc) + timedelta(minutes=5),
                    })
                    repository._items[match_item.id] = updated_item

                    # Record decision for undo
                    session_state.record_decision(match_item, DecisionType.ACCEPT, batch_id)

                    # Record feedback
                    await feedback_manager.record_accept(
                        match_item, session.id, batch_id=batch_id
                    )

                # Update session stats
                updated_session = session.model_copy(update={
                    "items_reviewed": session.items_reviewed + len(matching_items),
                    "items_accepted": session.items_accepted + len(matching_items),
                    "last_activity_at": datetime.now(timezone.utc),
                })
                await repository.update_session(updated_session)

                console.print(f"\n[green]Accepted {len(matching_items)} items.[/green]")
                console.print(f"[dim]Batch ID: {str(batch_id)[:8]}... (undo within 5 min)[/dim]")

                # Show next item
                await _show_next_item(ctx, updated_session)

            else:
                # Single item mode
                # Update item status
                updated_item = item.model_copy(update={
                    "status": ReviewItemStatus.ACCEPTED,
                    "user_decision": DecisionType.ACCEPT,
                    "reviewed_at": datetime.now(timezone.utc),
                    "undo_eligible": True,
                    "undo_deadline": datetime.now(timezone.utc) + timedelta(minutes=5),
                })
                repository._items[item.id] = updated_item

                # Record decision for undo
                session_state.record_decision(item, DecisionType.ACCEPT)

                # Record feedback
                await feedback_manager.record_accept(item, session.id)

                # Update session stats
                updated_session = session.model_copy(update={
                    "items_reviewed": session.items_reviewed + 1,
                    "items_accepted": session.items_accepted + 1,
                    "current_position": session.current_position + 1,
                    "last_activity_at": datetime.now(timezone.utc),
                })
                await repository.update_session(updated_session)

                preview = _truncate_preview(item.content_preview)
                console.print(f"[green]Accepted:[/green] {preview}")

                # Show next item
                await _show_next_item(ctx, updated_session)

            return 0

        except ReviewError as e:
            display_error(e)
            return 1

        except Exception as e:
            console.print(f"\n[red]Unexpected error:[/red] {e}")
            return 1

        finally:
            from penf_lib.storage.connections import cleanup_connections
            await cleanup_connections()

    result = asyncio.run(_accept_async())
    sys.exit(result)


@review_group.command("reject")
@click.argument("item_id", type=int, required=False)
@click.option(
    "--reason",
    type=click.Choice([
        "wrong_category",
        "wrong_participants",
        "not_relevant",
        "spam",
        "duplicate",
        "other",
    ]),
    help="Rejection reason code",
)
@click.option(
    "--batch",
    type=click.Choice(["thread", "sender", "category"]),
    help="Batch mode: reject all items matching criteria",
)
@click.option(
    "--confirm",
    is_flag=True,
    help="Skip confirmation for batch operations",
)
@click.pass_context
def review_reject(
    ctx: click.Context,
    item_id: int | None,
    reason: str | None,
    batch: str | None,
    confirm: bool,
) -> None:
    """Reject AI suggestion for current or specified item.

    Marks the AI suggestion as incorrect. Optionally provide a reason
    code for learning system feedback.

    Examples:
        penf review reject                       # Reject current item
        penf review reject 42 --reason spam      # Reject item 42 as spam
        penf review reject --batch sender        # Reject all from sender
    """
    async def _reject_async() -> int:
        try:
            from rich.prompt import Confirm

            session, item, error = await _get_item_for_action(ctx, item_id)

            if error:
                console.print(f"[yellow]{error}[/yellow]")
                return 0 if session is None else 1

            assert session is not None
            assert item is not None

            repository = get_mock_repository()
            session_state = get_mock_session_state()
            feedback_manager = get_mock_feedback_manager()

            if batch:
                # Batch mode - find matching items
                matching_items: list[ReviewItemDTO] = []

                for repo_item in repository._items.values():
                    if repo_item.session_id != session.id:
                        continue
                    if repo_item.status != ReviewItemStatus.PENDING:
                        continue

                    if batch == "sender":
                        if (item.ai_suggestion.participants and
                            repo_item.ai_suggestion.participants and
                            item.ai_suggestion.participants[0] == repo_item.ai_suggestion.participants[0]):
                            matching_items.append(repo_item)
                    elif batch == "category":
                        if item.ai_suggestion.category == repo_item.ai_suggestion.category:
                            matching_items.append(repo_item)
                    elif batch == "thread":
                        if item.source_id == repo_item.source_id:
                            matching_items.append(repo_item)

                if not matching_items:
                    console.print("[yellow]No matching items found for batch operation.[/yellow]")
                    return 0

                # Show preview
                console.print(f"\n[bold]Batch Reject - {len(matching_items)} items[/bold]")
                console.print(f"[dim]Matching by: {batch}[/dim]")
                if reason:
                    console.print(f"[dim]Reason: {reason}[/dim]")
                console.print()

                for idx, match_item in enumerate(matching_items[:5], 1):
                    preview = _truncate_preview(match_item.content_preview)
                    console.print(f"  {idx}. [{match_item.content_type.value}] {preview}")

                if len(matching_items) > 5:
                    console.print(f"  ... and {len(matching_items) - 5} more")

                # Require confirmation
                if not confirm:
                    console.print()
                    if not Confirm.ask(f"Reject all {len(matching_items)} items?", default=False):
                        console.print("[dim]Batch operation cancelled.[/dim]")
                        return 0

                # Apply batch operation
                batch_id = session_state.generate_batch_id()
                correction = UserCorrection(reason=reason) if reason else None

                for match_item in matching_items:
                    updated_item = match_item.model_copy(update={
                        "status": ReviewItemStatus.REJECTED,
                        "user_decision": DecisionType.REJECT,
                        "user_correction": correction,
                        "reviewed_at": datetime.now(timezone.utc),
                        "batch_id": batch_id,
                        "undo_eligible": True,
                        "undo_deadline": datetime.now(timezone.utc) + timedelta(minutes=5),
                    })
                    repository._items[match_item.id] = updated_item

                    session_state.record_decision(match_item, DecisionType.REJECT, batch_id, correction)
                    await feedback_manager.record_reject(
                        match_item, session.id, reason=reason, batch_id=batch_id
                    )

                # Update session stats
                updated_session = session.model_copy(update={
                    "items_reviewed": session.items_reviewed + len(matching_items),
                    "items_rejected": session.items_rejected + len(matching_items),
                    "last_activity_at": datetime.now(timezone.utc),
                })
                await repository.update_session(updated_session)

                console.print(f"\n[red]Rejected {len(matching_items)} items.[/red]")
                console.print(f"[dim]Batch ID: {str(batch_id)[:8]}... (undo within 5 min)[/dim]")

                # Show next item
                await _show_next_item(ctx, updated_session)

            else:
                # Single item mode
                correction = UserCorrection(reason=reason) if reason else None

                updated_item = item.model_copy(update={
                    "status": ReviewItemStatus.REJECTED,
                    "user_decision": DecisionType.REJECT,
                    "user_correction": correction,
                    "reviewed_at": datetime.now(timezone.utc),
                    "undo_eligible": True,
                    "undo_deadline": datetime.now(timezone.utc) + timedelta(minutes=5),
                })
                repository._items[item.id] = updated_item

                session_state.record_decision(item, DecisionType.REJECT, correction=correction)
                await feedback_manager.record_reject(item, session.id, reason=reason)

                # Update session stats
                updated_session = session.model_copy(update={
                    "items_reviewed": session.items_reviewed + 1,
                    "items_rejected": session.items_rejected + 1,
                    "current_position": session.current_position + 1,
                    "last_activity_at": datetime.now(timezone.utc),
                })
                await repository.update_session(updated_session)

                preview = _truncate_preview(item.content_preview)
                reason_str = f" ({reason})" if reason else ""
                console.print(f"[red]Rejected{reason_str}:[/red] {preview}")

                # Show next item
                await _show_next_item(ctx, updated_session)

            return 0

        except ReviewError as e:
            display_error(e)
            return 1

        except Exception as e:
            console.print(f"\n[red]Unexpected error:[/red] {e}")
            return 1

        finally:
            from penf_lib.storage.connections import cleanup_connections
            await cleanup_connections()

    result = asyncio.run(_reject_async())
    sys.exit(result)


@review_group.command("modify")
@click.argument("item_id", type=int, required=False)
@click.option("--category", help="Override category")
@click.option("--add-tag", multiple=True, help="Add tag (repeatable)")
@click.option("--remove-tag", multiple=True, help="Remove tag (repeatable)")
@click.option("--add-participant", multiple=True, help="Add participant (repeatable)")
@click.option("--remove-participant", multiple=True, help="Remove participant (repeatable)")
@click.option("--notes", help="Add correction notes")
@click.option(
    "--interactive/--no-interactive",
    default=True,
    help="Interactive modification mode (default: interactive)",
)
@click.pass_context
def review_modify(
    ctx: click.Context,
    item_id: int | None,
    category: str | None,
    add_tag: tuple[str, ...],
    remove_tag: tuple[str, ...],
    add_participant: tuple[str, ...],
    remove_participant: tuple[str, ...],
    notes: str | None,
    interactive: bool,
) -> None:
    """Modify AI suggestion for current or specified item.

    Allows correction of category, participants, or tags. Use interactive
    mode (default) for a guided editing experience.

    Examples:
        penf review modify                           # Interactive mode
        penf review modify --category finance/budget # Direct category change
        penf review modify --add-tag urgent          # Add a tag
        penf review modify --no-interactive --category ops  # Non-interactive
    """
    async def _modify_async() -> int:
        try:
            from rich.prompt import Prompt

            session, item, error = await _get_item_for_action(ctx, item_id)

            if error:
                console.print(f"[yellow]{error}[/yellow]")
                return 0 if session is None else 1

            assert session is not None
            assert item is not None

            repository = get_mock_repository()
            session_state = get_mock_session_state()
            feedback_manager = get_mock_feedback_manager()

            # Start with current suggestion values
            new_category = item.ai_suggestion.category
            new_tags = list(item.ai_suggestion.tags)
            new_participants = list(item.ai_suggestion.participants)
            correction_notes = notes

            # Check if any direct options provided
            has_direct_options = (
                category is not None or
                add_tag or remove_tag or
                add_participant or remove_participant
            )

            if interactive and not has_direct_options:
                # Interactive modification mode
                console.print("\n[bold]Modifying Item[/bold]")
                console.print("=" * 40)

                # Show current suggestion
                console.print("\n[bold]Current suggestion:[/bold]")
                console.print(f"  Category: [cyan]{item.ai_suggestion.category}[/cyan]")
                if item.ai_suggestion.participants:
                    console.print(f"  Participants: {', '.join(item.ai_suggestion.participants)}")
                if item.ai_suggestion.tags:
                    console.print(f"  Tags: {', '.join(f'#{t}' for t in item.ai_suggestion.tags)}")

                console.print("\n[dim]Options:[/dim]")
                console.print("  [c] Change category")
                console.print("  [p] Edit participants")
                console.print("  [t] Edit tags")
                console.print("  [n] Add notes")
                console.print("  [Enter] Apply changes")
                console.print("  [q] Cancel")

                modified = False
                while True:
                    console.print()
                    choice = Prompt.ask(
                        "Action",
                        choices=["c", "p", "t", "n", "", "q"],
                        default="",
                        show_choices=False,
                    )

                    if choice == "" or choice is None:
                        # Apply changes
                        break
                    elif choice == "q":
                        console.print("[dim]Modification cancelled.[/dim]")
                        return 0
                    elif choice == "c":
                        new_cat = Prompt.ask(
                            "New category",
                            default=new_category,
                        )
                        if new_cat and new_cat != new_category:
                            new_category = new_cat
                            modified = True
                            console.print(f"[green]Category set to: {new_category}[/green]")
                    elif choice == "p":
                        console.print(f"[dim]Current: {', '.join(new_participants) or 'none'}[/dim]")
                        action = Prompt.ask(
                            "[a]dd or [r]emove participant",
                            choices=["a", "r"],
                            default="a",
                        )
                        if action == "a":
                            name = Prompt.ask("Participant name")
                            if name and name not in new_participants:
                                new_participants.append(name)
                                modified = True
                                console.print(f"[green]Added: {name}[/green]")
                        else:
                            if new_participants:
                                name = Prompt.ask(
                                    "Remove",
                                    choices=new_participants,
                                )
                                if name in new_participants:
                                    new_participants.remove(name)
                                    modified = True
                                    console.print(f"[red]Removed: {name}[/red]")
                    elif choice == "t":
                        console.print(f"[dim]Current: {', '.join(f'#{t}' for t in new_tags) or 'none'}[/dim]")
                        action = Prompt.ask(
                            "[a]dd or [r]emove tag",
                            choices=["a", "r"],
                            default="a",
                        )
                        if action == "a":
                            tag = Prompt.ask("Tag name (without #)")
                            if tag and tag not in new_tags:
                                new_tags.append(tag)
                                modified = True
                                console.print(f"[green]Added: #{tag}[/green]")
                        else:
                            if new_tags:
                                tag = Prompt.ask(
                                    "Remove",
                                    choices=new_tags,
                                )
                                if tag in new_tags:
                                    new_tags.remove(tag)
                                    modified = True
                                    console.print(f"[red]Removed: #{tag}[/red]")
                    elif choice == "n":
                        correction_notes = Prompt.ask(
                            "Notes",
                            default=correction_notes or "",
                        )
                        if correction_notes:
                            modified = True
                            console.print("[green]Notes added[/green]")

                if not modified and not correction_notes:
                    console.print("[dim]No changes made.[/dim]")
                    return 0

            else:
                # Direct options mode (non-interactive or options provided)
                if category:
                    new_category = category

                for tag in add_tag:
                    if tag not in new_tags:
                        new_tags.append(tag)

                for tag in remove_tag:
                    if tag in new_tags:
                        new_tags.remove(tag)

                for participant in add_participant:
                    if participant not in new_participants:
                        new_participants.append(participant)

                for participant in remove_participant:
                    if participant in new_participants:
                        new_participants.remove(participant)

            # Build correction object
            correction = UserCorrection(
                category=new_category if new_category != item.ai_suggestion.category else None,
                participants=new_participants if new_participants != item.ai_suggestion.participants else None,
                tags=new_tags if new_tags != item.ai_suggestion.tags else None,
                notes=correction_notes,
            )

            # Update the AI suggestion with corrected values
            updated_suggestion = AISuggestion(
                category=new_category,
                participants=new_participants,
                tags=new_tags,
            )

            # Update item
            updated_item = item.model_copy(update={
                "status": ReviewItemStatus.MODIFIED,
                "user_decision": DecisionType.MODIFY,
                "user_correction": correction,
                "ai_suggestion": updated_suggestion,
                "reviewed_at": datetime.now(timezone.utc),
                "undo_eligible": True,
                "undo_deadline": datetime.now(timezone.utc) + timedelta(minutes=5),
            })
            repository._items[item.id] = updated_item

            # Record decision for undo
            session_state.record_decision(item, DecisionType.MODIFY, correction=correction)

            # Record feedback
            await feedback_manager.record_modify(item, session.id, correction)

            # Update session stats
            updated_session = session.model_copy(update={
                "items_reviewed": session.items_reviewed + 1,
                "items_modified": session.items_modified + 1,
                "current_position": session.current_position + 1,
                "last_activity_at": datetime.now(timezone.utc),
            })
            await repository.update_session(updated_session)

            preview = _truncate_preview(item.content_preview)
            console.print(f"[cyan]Modified:[/cyan] {preview}")

            # Show what changed
            changes = []
            if correction.category:
                changes.append(f"category={correction.category}")
            if correction.tags:
                changes.append(f"tags={len(correction.tags)}")
            if correction.participants:
                changes.append(f"participants={len(correction.participants)}")
            if changes:
                console.print(f"[dim]Changes: {', '.join(changes)}[/dim]")

            # Show next item
            await _show_next_item(ctx, updated_session)

            return 0

        except ReviewError as e:
            display_error(e)
            return 1

        except Exception as e:
            console.print(f"\n[red]Unexpected error:[/red] {e}")
            return 1

        finally:
            from penf_lib.storage.connections import cleanup_connections
            await cleanup_connections()

    result = asyncio.run(_modify_async())
    sys.exit(result)


@review_group.command("skip")
@click.argument("item_id", type=int, required=False)
@click.option("--reason", help="Reason for skipping")
@click.pass_context
def review_skip(
    ctx: click.Context,
    item_id: int | None,
    reason: str | None,
) -> None:
    """Skip current item for later review.

    Moves the item to the end of the queue for later review.
    Optionally provide a reason for skipping.

    Examples:
        penf review skip                        # Skip current item
        penf review skip 42                     # Skip item 42
        penf review skip --reason "need info"   # Skip with reason
    """
    async def _skip_async() -> int:
        try:
            session, item, error = await _get_item_for_action(ctx, item_id)

            if error:
                console.print(f"[yellow]{error}[/yellow]")
                return 0 if session is None else 1

            assert session is not None
            assert item is not None

            repository = get_mock_repository()
            session_state = get_mock_session_state()
            feedback_manager = get_mock_feedback_manager()

            # Update item status
            correction = UserCorrection(notes=reason) if reason else None
            updated_item = item.model_copy(update={
                "status": ReviewItemStatus.SKIPPED,
                "user_decision": DecisionType.SKIP,
                "user_correction": correction,
                "reviewed_at": datetime.now(timezone.utc),
                "undo_eligible": True,
                "undo_deadline": datetime.now(timezone.utc) + timedelta(minutes=5),
                # Move to end of queue
                "queue_position": session.total_items + 1,
            })
            repository._items[item.id] = updated_item

            # Record decision for undo
            session_state.record_decision(item, DecisionType.SKIP, correction=correction)

            # Record feedback
            await feedback_manager.record_skip(item, session.id, reason=reason)

            # Update session stats
            updated_session = session.model_copy(update={
                "items_reviewed": session.items_reviewed + 1,
                "items_skipped": session.items_skipped + 1,
                "current_position": session.current_position + 1,
                "last_activity_at": datetime.now(timezone.utc),
            })
            await repository.update_session(updated_session)

            preview = _truncate_preview(item.content_preview)
            reason_str = f" ({reason})" if reason else ""
            console.print(f"[yellow]Skipped{reason_str}:[/yellow] {preview}")

            # Show next item
            await _show_next_item(ctx, updated_session)

            return 0

        except ReviewError as e:
            display_error(e)
            return 1

        except Exception as e:
            console.print(f"\n[red]Unexpected error:[/red] {e}")
            return 1

        finally:
            from penf_lib.storage.connections import cleanup_connections
            await cleanup_connections()

    result = asyncio.run(_skip_async())
    sys.exit(result)


@review_group.command("undo")
@click.option(
    "--count",
    type=int,
    default=1,
    help="Number of decisions to undo (default: 1)",
)
@click.option(
    "--batch-id",
    type=click.UUID,
    help="Undo specific batch operation by ID",
)
@click.pass_context
def review_undo(
    ctx: click.Context,
    count: int,
    batch_id: UUID | None,
) -> None:
    """Undo recent review decision(s).

    Reverses the most recent decision(s) and restores items to pending.
    Use --batch-id to undo an entire batch operation.

    Examples:
        penf review undo             # Undo last decision
        penf review undo --count 3   # Undo last 3 decisions
        penf review undo --batch-id abc123...  # Undo specific batch
    """
    async def _undo_async() -> int:
        try:
            from rich.prompt import Confirm

            # Get tenant context
            tenant_id_str = ctx.obj.get("tenant_id")
            user_email = ctx.obj.get("user_email")

            if not tenant_id_str:
                tenant_id = UUID("00000000-0000-0000-0000-000000000001")
            else:
                tenant_id = UUID(tenant_id_str)

            if not user_email:
                user_email = "developer@example.com"

            repository = get_mock_repository()
            session_manager = SessionManager(repository)
            session_state = get_mock_session_state()

            session = await session_manager.get_active_session(tenant_id, user_email)

            if not session:
                console.print("[yellow]No active review session.[/yellow]")
                console.print("\n[dim]Start a review with: penf review[/dim]")
                return 0

            # Get decisions to undo
            if batch_id:
                decisions = session_state.get_decisions_by_batch(batch_id)
                if not decisions:
                    console.print(f"[yellow]No decisions found for batch {str(batch_id)[:8]}...[/yellow]")
                    return 0
            else:
                decisions = session_state.get_recent_decisions(count)
                if not decisions:
                    console.print("[yellow]No decisions to undo.[/yellow]")
                    return 0

            # Check undo eligibility
            now = datetime.now(timezone.utc)
            eligible_decisions = []
            for decision in decisions:
                item = repository._items.get(decision["item_id"])
                if item and item.undo_eligible:
                    if item.undo_deadline is None or item.undo_deadline > now:
                        eligible_decisions.append(decision)
                    else:
                        console.print(f"[yellow]Item {item.id}: undo deadline passed[/yellow]")
                else:
                    console.print(f"[yellow]Item {decision['item_id']}: not eligible for undo[/yellow]")

            if not eligible_decisions:
                console.print("[yellow]No eligible decisions to undo.[/yellow]")
                return 0

            # Show confirmation
            console.print(f"\n[bold]Undo {len(eligible_decisions)} decision(s)?[/bold]\n")

            for decision in eligible_decisions:
                item_id = decision["item_id"]
                item = repository._items.get(item_id)
                if item:
                    preview = _truncate_preview(item.content_preview)
                    decision_type = decision["decision"].value
                    console.print(f"  Item {item_id}: {item.content_type.value} \"{preview}\" - {decision_type}")

            console.print()
            if not Confirm.ask("Proceed?", default=False):
                console.print("[dim]Undo cancelled.[/dim]")
                return 0

            # Perform undo
            accept_undone = 0
            reject_undone = 0
            modify_undone = 0
            skip_undone = 0

            for decision in eligible_decisions:
                item_id = decision["item_id"]
                original_item = decision["item"]

                # Restore item to original state
                restored_item = original_item.model_copy(update={
                    "status": ReviewItemStatus.PENDING,
                    "user_decision": None,
                    "user_correction": None,
                    "reviewed_at": None,
                    "batch_id": None,
                    "undo_eligible": True,
                    "undo_deadline": None,
                })
                repository._items[item_id] = restored_item

                # Track what was undone
                if decision["decision"] == DecisionType.ACCEPT:
                    accept_undone += 1
                elif decision["decision"] == DecisionType.REJECT:
                    reject_undone += 1
                elif decision["decision"] == DecisionType.MODIFY:
                    modify_undone += 1
                elif decision["decision"] == DecisionType.SKIP:
                    skip_undone += 1

            # Remove decisions from history
            if batch_id:
                # Remove all decisions with this batch_id
                session_state._decision_history = [
                    d for d in session_state._decision_history
                    if d.get("batch_id") != batch_id
                ]
            else:
                session_state.pop_decisions(len(eligible_decisions))

            # Update session stats
            updated_session = session.model_copy(update={
                "items_reviewed": max(0, session.items_reviewed - len(eligible_decisions)),
                "items_accepted": max(0, session.items_accepted - accept_undone),
                "items_rejected": max(0, session.items_rejected - reject_undone),
                "items_modified": max(0, session.items_modified - modify_undone),
                "items_skipped": max(0, session.items_skipped - skip_undone),
                "current_position": max(0, session.current_position - len(eligible_decisions)),
                "last_activity_at": datetime.now(timezone.utc),
            })
            await repository.update_session(updated_session)

            console.print(f"\n[green]Undone {len(eligible_decisions)} decision(s).[/green]")

            # Show the first undone item
            if eligible_decisions:
                first_item_id = eligible_decisions[0]["item_id"]
                first_item = repository._items.get(first_item_id)
                if first_item:
                    session_state.current_item_id = first_item_id
                    console.print()
                    render_item(
                        console,
                        first_item,
                        position=updated_session.current_position + 1,
                        total=updated_session.total_items,
                    )

            return 0

        except ReviewError as e:
            display_error(e)
            return 1

        except Exception as e:
            console.print(f"\n[red]Unexpected error:[/red] {e}")
            return 1

        finally:
            from penf_lib.storage.connections import cleanup_connections
            await cleanup_connections()

    result = asyncio.run(_undo_async())
    sys.exit(result)


# =============================================================================
# BATCH AND COMPLETION COMMANDS (User Story 3)
# =============================================================================


class MockBatchManager:
    """Mock batch manager for development.

    Provides batch operations for finding and processing similar items
    during the review workflow.
    """

    def __init__(self, repository: MockReviewRepository) -> None:
        """Initialize batch manager with repository.

        Args:
            repository: The review repository instance
        """
        self._repository = repository

    async def find_similar_items(
        self,
        session_id: int,
        reference_item: ReviewItemDTO,
        group_by: str,
        filter_expr: str | None = None,
    ) -> list[ReviewItemDTO]:
        """Find items similar to the reference item.

        Args:
            session_id: Current session ID
            reference_item: Item to match against
            group_by: Grouping mode (thread, sender, category, time)
            filter_expr: Optional filter expression

        Returns:
            List of matching items
        """
        matching_items: list[ReviewItemDTO] = []

        for item in self._repository._items.values():
            if item.session_id != session_id:
                continue
            if item.status != ReviewItemStatus.PENDING:
                continue

            # Apply grouping match
            matches = False
            if group_by == "sender":
                # Match by first participant (sender)
                if (reference_item.ai_suggestion.participants and
                    item.ai_suggestion.participants and
                    reference_item.ai_suggestion.participants[0] == item.ai_suggestion.participants[0]):
                    matches = True
            elif group_by == "category":
                # Match by category
                if reference_item.ai_suggestion.category == item.ai_suggestion.category:
                    matches = True
            elif group_by == "thread":
                # Match by source_id (thread grouping)
                if reference_item.source_id == item.source_id:
                    matches = True
            elif group_by == "time":
                # Match by source timestamp within same day
                if (reference_item.source_timestamp and item.source_timestamp):
                    ref_date = reference_item.source_timestamp.date()
                    item_date = item.source_timestamp.date()
                    if ref_date == item_date:
                        matches = True

            if matches:
                matching_items.append(item)

        # Apply filter expression if provided
        if filter_expr:
            matching_items = self._apply_filter(filter_expr, matching_items)

        return matching_items

    def _apply_filter(
        self,
        filter_expr: str,
        items: list[ReviewItemDTO],
    ) -> list[ReviewItemDTO]:
        """Apply filter expression to items.

        Supports simple expressions like:
        - confidence>0.8
        - confidence<0.5
        - type=email
        - type=meeting

        Args:
            filter_expr: Filter expression string
            items: Items to filter

        Returns:
            Filtered items
        """
        if filter_expr.startswith("confidence"):
            try:
                if ">" in filter_expr:
                    op_idx = filter_expr.index(">")
                    val = float(filter_expr[op_idx + 1:])
                    return [i for i in items if float(i.ai_confidence) > val]
                elif "<" in filter_expr:
                    op_idx = filter_expr.index("<")
                    val = float(filter_expr[op_idx + 1:])
                    return [i for i in items if float(i.ai_confidence) < val]
                elif ">=" in filter_expr:
                    op_idx = filter_expr.index(">=")
                    val = float(filter_expr[op_idx + 2:])
                    return [i for i in items if float(i.ai_confidence) >= val]
                elif "<=" in filter_expr:
                    op_idx = filter_expr.index("<=")
                    val = float(filter_expr[op_idx + 2:])
                    return [i for i in items if float(i.ai_confidence) <= val]
            except (ValueError, IndexError):
                pass
        elif filter_expr.startswith("type="):
            type_val = filter_expr[5:].strip().lower()
            return [i for i in items if i.content_type.value.lower() == type_val]

        return items

    async def execute_batch(
        self,
        session_id: int,
        items: list[ReviewItemDTO],
        action: str,
        reason: str | None = None,
    ) -> tuple[UUID, int]:
        """Execute batch action on items.

        Args:
            session_id: Current session ID
            items: Items to process
            action: Action to apply (accept, reject, skip)
            reason: Optional reason for reject/skip

        Returns:
            Tuple of (batch_id, count_processed)
        """
        session_state = get_mock_session_state()
        feedback_manager = get_mock_feedback_manager()
        batch_id = session_state.generate_batch_id()
        now = datetime.now(timezone.utc)

        for item in items:
            if action == "accept":
                status = ReviewItemStatus.ACCEPTED
                decision = DecisionType.ACCEPT
                correction = None
            elif action == "reject":
                status = ReviewItemStatus.REJECTED
                decision = DecisionType.REJECT
                correction = UserCorrection(reason=reason) if reason else None
            else:  # skip
                status = ReviewItemStatus.SKIPPED
                decision = DecisionType.SKIP
                correction = UserCorrection(notes=reason) if reason else None

            # Update item
            updated_item = item.model_copy(update={
                "status": status,
                "user_decision": decision,
                "user_correction": correction,
                "reviewed_at": now,
                "batch_id": batch_id,
                "undo_eligible": True,
                "undo_deadline": now + timedelta(minutes=5),
            })
            self._repository._items[item.id] = updated_item

            # Record for undo
            session_state.record_decision(item, decision, batch_id, correction)

            # Record feedback
            if action == "accept":
                await feedback_manager.record_accept(item, session_id, batch_id=batch_id)
            elif action == "reject":
                await feedback_manager.record_reject(
                    item, session_id, reason=reason, batch_id=batch_id
                )
            else:  # skip
                await feedback_manager.record_skip(item, session_id, reason=reason)

        return batch_id, len(items)


def get_mock_batch_manager() -> MockBatchManager:
    """Get a batch manager instance."""
    return MockBatchManager(get_mock_repository())


@review_group.command("batch")
@click.option(
    "--group",
    type=click.Choice(["thread", "sender", "category", "time"]),
    required=True,
    help="Group items by: thread, sender, category, or time",
)
@click.option(
    "--action",
    type=click.Choice(["accept", "reject", "skip"]),
    required=True,
    help="Action to apply: accept, reject, or skip",
)
@click.option("--filter", "filter_expr", help="Filter expression (e.g., 'confidence>0.8')")
@click.option("--preview/--no-preview", default=True, help="Show preview before applying")
@click.option("--reason", help="Reason for reject/skip actions")
@click.pass_context
def review_batch(
    ctx: click.Context,
    group: str,
    action: str,
    filter_expr: str | None,
    preview: bool,
    reason: str | None,
) -> None:
    """Apply batch operations to multiple similar items.

    Groups items by a specified criteria and applies the same action
    to all matching items. Use --preview (default) to see affected
    items before applying.

    Examples:
        penf review batch --group sender --action accept
        penf review batch --group category --action reject --reason spam
        penf review batch --group thread --action skip --no-preview
        penf review batch --group time --action accept --filter "confidence>0.7"
    """
    async def _batch_async() -> int:
        try:
            from rich.prompt import Confirm

            # Get tenant context
            tenant_id_str = ctx.obj.get("tenant_id")
            user_email = ctx.obj.get("user_email")

            if not tenant_id_str:
                tenant_id = UUID("00000000-0000-0000-0000-000000000001")
            else:
                tenant_id = UUID(tenant_id_str)

            if not user_email:
                user_email = "developer@example.com"

            repository = get_mock_repository()
            session_manager = SessionManager(repository)
            batch_manager = get_mock_batch_manager()
            session_state = get_mock_session_state()

            session = await session_manager.get_active_session(tenant_id, user_email)

            if not session:
                console.print("[yellow]No active review session.[/yellow]")
                console.print("\n[dim]Start a review with: penf review[/dim]")
                return 0

            # Get reference item (current item or first pending)
            reference_item: ReviewItemDTO | None = None

            if session_state.current_item_id:
                reference_item = repository._items.get(session_state.current_item_id)

            if not reference_item:
                # Find first pending item in session
                for item in repository._items.values():
                    if item.session_id == session.id and item.status == ReviewItemStatus.PENDING:
                        reference_item = item
                        break

            if not reference_item:
                console.print("[yellow]No pending items for batch operation.[/yellow]")
                return 0

            # Find matching items
            matching_items = await batch_manager.find_similar_items(
                session_id=session.id,
                reference_item=reference_item,
                group_by=group,
                filter_expr=filter_expr,
            )

            if not matching_items:
                console.print("[yellow]No matching items found for batch operation.[/yellow]")
                return 0

            # Determine grouping label
            group_label = ""
            if group == "sender" and reference_item.ai_suggestion.participants:
                group_label = f" ({reference_item.ai_suggestion.participants[0]})"
            elif group == "category":
                group_label = f" ({reference_item.ai_suggestion.category})"
            elif group == "thread":
                group_label = f" (source {reference_item.source_id})"
            elif group == "time" and reference_item.source_timestamp:
                group_label = f" ({reference_item.source_timestamp.date().isoformat()})"

            if preview:
                # Show preview
                console.print("\n[bold]Batch Operation Preview[/bold]")
                console.print("=" * 40)
                console.print()
                console.print(f"[bold]Grouping:[/bold] {group}{group_label}")
                console.print(f"[bold]Action:[/bold] {action}")
                if filter_expr:
                    console.print(f"[bold]Filter:[/bold] {filter_expr}")
                if reason:
                    console.print(f"[bold]Reason:[/bold] {reason}")
                console.print()
                console.print(f"[bold]Affected items ({len(matching_items)}):[/bold]")

                for idx, item in enumerate(matching_items[:10], 1):
                    preview_text = _truncate_preview(item.content_preview, 50)
                    console.print(f"  - Item {item.id}: {preview_text}")

                if len(matching_items) > 10:
                    console.print(f"  ... and {len(matching_items) - 10} more")

                console.print()
                if not Confirm.ask(f"Apply to all {len(matching_items)} items?", default=False):
                    console.print("[dim]Batch operation cancelled.[/dim]")
                    return 0

            # Execute batch
            batch_id, count = await batch_manager.execute_batch(
                session_id=session.id,
                items=matching_items,
                action=action,
                reason=reason,
            )

            # Update session stats
            if action == "accept":
                updated_session = session.model_copy(update={
                    "items_reviewed": session.items_reviewed + count,
                    "items_accepted": session.items_accepted + count,
                    "last_activity_at": datetime.now(timezone.utc),
                })
            elif action == "reject":
                updated_session = session.model_copy(update={
                    "items_reviewed": session.items_reviewed + count,
                    "items_rejected": session.items_rejected + count,
                    "last_activity_at": datetime.now(timezone.utc),
                })
            else:  # skip
                updated_session = session.model_copy(update={
                    "items_reviewed": session.items_reviewed + count,
                    "items_skipped": session.items_skipped + count,
                    "last_activity_at": datetime.now(timezone.utc),
                })

            await repository.update_session(updated_session)

            # Show result
            action_color = {"accept": "green", "reject": "red", "skip": "yellow"}[action]
            action_past = {"accept": "Accepted", "reject": "Rejected", "skip": "Skipped"}[action]
            console.print(f"\n[{action_color}]{action_past} {count} items.[/{action_color}]")
            console.print(f"[dim]Batch ID: {str(batch_id)[:8]}... (undo within 5 min)[/dim]")

            # Show next item
            await _show_next_item(ctx, updated_session)

            return 0

        except ReviewError as e:
            display_error(e)
            return 1

        except Exception as e:
            console.print(f"\n[red]Unexpected error:[/red] {e}")
            return 1

        finally:
            from penf_lib.storage.connections import cleanup_connections
            await cleanup_connections()

    result = asyncio.run(_batch_async())
    sys.exit(result)


@review_group.command("complete")
@click.option("--force", is_flag=True, help="Complete even with pending items")
@click.option("--summary/--no-summary", default=True, help="Show session summary")
@click.pass_context
def review_complete(
    ctx: click.Context,
    force: bool,
    summary: bool,
) -> None:
    """Complete the current review session.

    Marks the session as completed and optionally displays a summary
    of the review. Use --force to complete even if items remain pending.

    Examples:
        penf review complete              # Complete with summary
        penf review complete --no-summary # Complete without summary
        penf review complete --force      # Complete with pending items
    """
    async def _complete_async() -> int:
        try:
            from rich.prompt import Confirm

            # Get tenant context
            tenant_id_str = ctx.obj.get("tenant_id")
            user_email = ctx.obj.get("user_email")

            if not tenant_id_str:
                tenant_id = UUID("00000000-0000-0000-0000-000000000001")
            else:
                tenant_id = UUID(tenant_id_str)

            if not user_email:
                user_email = "developer@example.com"

            repository = get_mock_repository()
            session_manager = SessionManager(repository)

            session = await session_manager.get_active_session(tenant_id, user_email)

            if not session:
                console.print("[yellow]No active review session.[/yellow]")
                console.print("\n[dim]Start a review with: penf review[/dim]")
                return 0

            # Check for pending items
            pending_count = session.total_items - session.items_reviewed
            if pending_count > 0 and not force:
                console.print(f"\n[yellow]Warning: {pending_count} items still pending.[/yellow]")
                console.print()
                if not Confirm.ask("Complete session anyway?", default=False):
                    console.print("[dim]Session not completed.[/dim]")
                    console.print(f"\n[dim]Use 'penf review next' to continue reviewing, "
                                  f"or --force to complete with pending items.[/dim]")
                    return 0

            # Mark session as completed
            now = datetime.now(timezone.utc)
            completed_session = session.model_copy(update={
                "status": SessionStatus.COMPLETED,
                "completed_at": now,
                "last_activity_at": now,
            })
            await repository.update_session(completed_session)

            # Clear session state
            session_state = get_mock_session_state()
            session_state.current_item_id = None

            if summary:
                # Calculate session statistics
                duration_seconds = 0
                if completed_session.started_at:
                    duration_delta = now - completed_session.started_at
                    duration_seconds = int(duration_delta.total_seconds())

                duration_minutes = duration_seconds // 60

                # Calculate percentages
                total_reviewed = completed_session.items_reviewed
                if total_reviewed > 0:
                    accept_pct = (completed_session.items_accepted / total_reviewed) * 100
                    modified_pct = (completed_session.items_modified / total_reviewed) * 100
                    reject_pct = (completed_session.items_rejected / total_reviewed) * 100
                    skip_pct = (completed_session.items_skipped / total_reviewed) * 100
                else:
                    accept_pct = modified_pct = reject_pct = skip_pct = 0.0

                # Calculate average time per item
                if total_reviewed > 0 and duration_seconds > 0:
                    avg_seconds = duration_seconds / total_reviewed
                else:
                    avg_seconds = 0.0

                # Calculate AI accuracy estimate
                # Accuracy = accepted / (accepted + rejected + modified)
                # Skipped items are not counted as they weren't evaluated
                evaluated = (completed_session.items_accepted +
                             completed_session.items_rejected +
                             completed_session.items_modified)
                if evaluated > 0:
                    ai_accuracy = (completed_session.items_accepted / evaluated) * 100
                else:
                    ai_accuracy = 0.0

                # Display summary
                console.print("\n[bold]Review Session Complete[/bold]")
                console.print("=" * 40)
                console.print()
                console.print(f"[bold]Session Duration:[/bold] {duration_minutes} minutes")
                console.print(f"[bold]Items Reviewed:[/bold] {total_reviewed}")
                if pending_count > 0:
                    console.print(f"[bold]Items Pending:[/bold] {pending_count}")
                console.print()
                console.print("[bold]Summary:[/bold]")
                console.print(f"  [green]Accepted:[/green] {completed_session.items_accepted} ({accept_pct:.1f}%)")
                console.print(f"  [cyan]Modified:[/cyan] {completed_session.items_modified} ({modified_pct:.1f}%)")
                console.print(f"  [red]Rejected:[/red] {completed_session.items_rejected} ({reject_pct:.1f}%)")
                console.print(f"  [yellow]Skipped:[/yellow] {completed_session.items_skipped} ({skip_pct:.1f}%)")
                console.print()
                console.print(f"[bold]Average time per item:[/bold] {avg_seconds:.1f} seconds")
                console.print(f"[bold]AI accuracy (estimated):[/bold] {ai_accuracy:.0f}%")

            else:
                console.print("\n[green]Review session completed.[/green]")

            return 0

        except ReviewError as e:
            display_error(e)
            return 1

        except Exception as e:
            console.print(f"\n[red]Unexpected error:[/red] {e}")
            return 1

        finally:
            from penf_lib.storage.connections import cleanup_connections
            await cleanup_connections()

    result = asyncio.run(_complete_async())
    sys.exit(result)


# =============================================================================
# LEARNING RULE COMMANDS (User Story 4)
# =============================================================================


class RuleSuggestionDTO:
    """Data transfer object for pending rule suggestions.

    Represents a suggested rule that hasn't been accepted yet.
    Used during development/mock phase.
    """

    def __init__(
        self,
        id: int,
        rule_type: LearningRuleType,
        pattern: str,
        confidence: float,
        occurrences: int,
        suggested_action: dict[str, Any],
        sample_matches: list[str] | None = None,
        created_at: datetime | None = None,
    ) -> None:
        """Initialize a rule suggestion.

        Args:
            id: Suggestion identifier
            rule_type: Type of rule being suggested
            pattern: The pattern detected
            confidence: Confidence score (0-1)
            occurrences: Number of times pattern was seen
            suggested_action: Action to apply when matched
            sample_matches: Example content that matched
            created_at: When the suggestion was created
        """
        self.id = id
        self.rule_type = rule_type
        self.pattern = pattern
        self.confidence = confidence
        self.occurrences = occurrences
        self.suggested_action = suggested_action
        self.sample_matches = sample_matches or []
        self.created_at = created_at or datetime.now(timezone.utc)


class MockRuleManager:
    """Mock rule manager for development.

    Provides in-memory storage for learning rules and suggestions
    during development. Will be replaced by actual database repository.
    """

    def __init__(self) -> None:
        """Initialize mock rule manager with sample data."""
        self._rules: dict[int, LearningRuleDTO] = {}
        self._suggestions: dict[int, RuleSuggestionDTO] = {}
        self._rule_counter = 0
        self._suggestion_counter = 100  # Start at 100 to distinguish from rules
        self._generate_sample_rules()
        self._generate_sample_suggestions()

    def _generate_sample_rules(self) -> None:
        """Generate sample learning rules for testing."""
        now = datetime.now(timezone.utc)
        tenant_id = UUID("00000000-0000-0000-0000-000000000001")

        sample_rules = [
            {
                "name": "Newsletter sender categorization",
                "description": "Automatically categorize emails from newsletter@company.com",
                "rule_type": LearningRuleType.SENDER_MATCH,
                "match_criteria": {"sender": "newsletter@company.com"},
                "action": {"category": "marketing/newsletters"},
                "status": LearningRuleStatus.ACTIVE,
                "times_applied": 145,
                "times_overridden": 7,
                "effectiveness_score": Decimal("0.952"),
                "confidence_threshold": Decimal("0.800"),
                "priority": 1,
            },
            {
                "name": "Weekly standup meeting pattern",
                "description": "Match content containing 'Weekly Standup'",
                "rule_type": LearningRuleType.CONTENT_PATTERN,
                "match_criteria": {"pattern": "Weekly Standup"},
                "action": {"category": "meetings/standup"},
                "status": LearningRuleStatus.ACTIVE,
                "times_applied": 52,
                "times_overridden": 7,
                "effectiveness_score": Decimal("0.865"),
                "confidence_threshold": Decimal("0.750"),
                "priority": 10,
            },
            {
                "name": "System alerts sender",
                "description": "Categorize system alerts from alerts@system.com",
                "rule_type": LearningRuleType.SENDER_MATCH,
                "match_criteria": {"sender": "alerts@system.com"},
                "action": {"category": "operations/alerts"},
                "status": LearningRuleStatus.DISABLED,
                "times_applied": 89,
                "times_overridden": 25,
                "effectiveness_score": Decimal("0.719"),
                "confidence_threshold": Decimal("0.700"),
                "priority": 20,
            },
            {
                "name": "Budget review pattern",
                "description": "Match documents about budget reviews",
                "rule_type": LearningRuleType.CONTENT_PATTERN,
                "match_criteria": {"pattern": "Budget Review"},
                "action": {"category": "finance/budget"},
                "status": LearningRuleStatus.ACTIVE,
                "times_applied": 23,
                "times_overridden": 2,
                "effectiveness_score": Decimal("0.913"),
                "confidence_threshold": Decimal("0.850"),
                "priority": 5,
            },
        ]

        for data in sample_rules:
            self._rule_counter += 1
            rule = LearningRuleDTO(
                id=self._rule_counter,
                rule_uuid=uuid4(),
                tenant_id=tenant_id,
                created_by="developer@example.com",
                created_at=now - timedelta(days=10 - self._rule_counter),
                updated_at=now - timedelta(hours=self._rule_counter * 2),
                last_applied_at=now - timedelta(hours=self._rule_counter),
                **data,
            )
            self._rules[rule.id] = rule

    def _generate_sample_suggestions(self) -> None:
        """Generate sample rule suggestions for testing."""
        now = datetime.now(timezone.utc)

        sample_suggestions = [
            {
                "rule_type": LearningRuleType.SENDER_MATCH,
                "pattern": "john.doe@company.com",
                "confidence": 0.92,
                "occurrences": 15,
                "suggested_action": {"category": "team/john-doe"},
                "sample_matches": [
                    "Re: Project Update from John",
                    "John's Weekly Summary",
                    "Meeting notes from John",
                ],
            },
            {
                "rule_type": LearningRuleType.CONTENT_PATTERN,
                "pattern": "Budget Review",
                "confidence": 0.85,
                "occurrences": 8,
                "suggested_action": {"category": "finance/reviews"},
                "sample_matches": [
                    "Q1 Budget Review Meeting",
                    "Budget Review: January 2026",
                ],
            },
            {
                "rule_type": LearningRuleType.SENDER_MATCH,
                "pattern": "support@vendor.com",
                "confidence": 0.78,
                "occurrences": 6,
                "suggested_action": {"category": "vendor/support"},
                "sample_matches": [
                    "Ticket #12345 Updated",
                    "Support Case Resolution",
                ],
            },
        ]

        for data in sample_suggestions:
            self._suggestion_counter += 1
            suggestion = RuleSuggestionDTO(
                id=self._suggestion_counter,
                created_at=now - timedelta(hours=self._suggestion_counter - 100),
                **data,
            )
            self._suggestions[suggestion.id] = suggestion

    async def get_rules(
        self,
        status: str | None = None,
    ) -> list[LearningRuleDTO]:
        """Get learning rules, optionally filtered by status.

        Args:
            status: Filter by status (active, disabled, all)

        Returns:
            List of matching rules
        """
        rules = list(self._rules.values())

        if status and status != "all":
            status_enum = LearningRuleStatus(status)
            rules = [r for r in rules if r.status == status_enum]

        # Sort by priority (lower = higher priority)
        rules.sort(key=lambda r: (r.priority, r.id))
        return rules

    async def get_rule_by_id(self, rule_id: int) -> LearningRuleDTO | None:
        """Get a specific rule by ID.

        Args:
            rule_id: Rule identifier

        Returns:
            The rule or None if not found
        """
        return self._rules.get(rule_id)

    async def enable_rule(self, rule_id: int) -> LearningRuleDTO | None:
        """Enable a disabled rule.

        Args:
            rule_id: Rule identifier

        Returns:
            Updated rule or None if not found
        """
        rule = self._rules.get(rule_id)
        if rule:
            updated = LearningRuleDTO(
                **{
                    **rule.model_dump(),
                    "status": LearningRuleStatus.ACTIVE,
                    "updated_at": datetime.now(timezone.utc),
                }
            )
            self._rules[rule_id] = updated
            return updated
        return None

    async def disable_rule(self, rule_id: int) -> LearningRuleDTO | None:
        """Disable an active rule.

        Args:
            rule_id: Rule identifier

        Returns:
            Updated rule or None if not found
        """
        rule = self._rules.get(rule_id)
        if rule:
            updated = LearningRuleDTO(
                **{
                    **rule.model_dump(),
                    "status": LearningRuleStatus.DISABLED,
                    "updated_at": datetime.now(timezone.utc),
                }
            )
            self._rules[rule_id] = updated
            return updated
        return None

    async def get_pending_suggestions(self) -> list[RuleSuggestionDTO]:
        """Get all pending rule suggestions.

        Returns:
            List of pending suggestions sorted by confidence
        """
        suggestions = list(self._suggestions.values())
        suggestions.sort(key=lambda s: (-s.confidence, -s.occurrences))
        return suggestions

    async def get_suggestion_by_id(
        self, suggestion_id: int
    ) -> RuleSuggestionDTO | None:
        """Get a specific suggestion by ID.

        Args:
            suggestion_id: Suggestion identifier

        Returns:
            The suggestion or None if not found
        """
        return self._suggestions.get(suggestion_id)

    async def accept_suggestion(
        self, suggestion_id: int
    ) -> LearningRuleDTO | None:
        """Accept a suggestion and create a new rule from it.

        Args:
            suggestion_id: Suggestion identifier

        Returns:
            The created rule or None if suggestion not found
        """
        suggestion = self._suggestions.get(suggestion_id)
        if not suggestion:
            return None

        # Remove from suggestions
        del self._suggestions[suggestion_id]

        # Create new rule
        self._rule_counter += 1
        now = datetime.now(timezone.utc)
        tenant_id = UUID("00000000-0000-0000-0000-000000000001")

        rule = LearningRuleDTO(
            id=self._rule_counter,
            rule_uuid=uuid4(),
            tenant_id=tenant_id,
            name=f"Rule from suggestion: {suggestion.pattern[:30]}...",
            description=f"Auto-generated from suggestion #{suggestion_id}",
            status=LearningRuleStatus.ACTIVE,
            rule_type=suggestion.rule_type,
            match_criteria={"pattern": suggestion.pattern},
            action=suggestion.suggested_action,
            confidence_threshold=Decimal(str(suggestion.confidence)),
            priority=50,
            derived_from_count=suggestion.occurrences,
            times_applied=0,
            times_overridden=0,
            effectiveness_score=None,
            last_applied_at=None,
            expires_at=None,
            created_by="developer@example.com",
            created_at=now,
            updated_at=now,
            is_deleted=False,
            deleted_at=None,
        )
        self._rules[rule.id] = rule
        return rule

    async def reject_suggestion(self, suggestion_id: int) -> bool:
        """Reject a suggestion.

        Args:
            suggestion_id: Suggestion identifier

        Returns:
            True if suggestion was found and rejected
        """
        if suggestion_id in self._suggestions:
            del self._suggestions[suggestion_id]
            return True
        return False


# Global mock rule manager instance
_mock_rule_manager: MockRuleManager | None = None


def get_mock_rule_manager() -> MockRuleManager:
    """Get or create the mock rule manager singleton."""
    global _mock_rule_manager
    if _mock_rule_manager is None:
        _mock_rule_manager = MockRuleManager()
    return _mock_rule_manager


# =============================================================================
# RULES SUBGROUP
# =============================================================================


@review_group.group("rules")
@click.pass_context
def rules_group(ctx: click.Context) -> None:
    """Manage learning rules for automated corrections.

    Learning rules are patterns derived from your review decisions
    that can automatically apply corrections to future content.

    Examples:
        penf review rules list              # List all rules
        penf review rules show 1            # Show rule details
        penf review rules enable 3          # Enable a rule
        penf review rules disable 2         # Disable a rule
        penf review rules pending           # View pending suggestions
        penf review rules accept 101        # Accept a suggestion
    """
    ctx.ensure_object(dict)


@rules_group.command("list")
@click.option(
    "--status",
    type=click.Choice(["active", "disabled", "all"]),
    default="all",
    help="Filter rules by status",
)
@click.pass_context
def rules_list(ctx: click.Context, status: str) -> None:
    """List learning rules.

    Shows all learning rules with their type, pattern, action,
    status, and effectiveness score.

    Examples:
        penf review rules list                 # All rules
        penf review rules list --status active # Only active rules
    """
    from rich.table import Table

    async def _list_async() -> int:
        try:
            rule_manager = get_mock_rule_manager()
            rules = await rule_manager.get_rules(status=status)

            if not rules:
                if status == "all":
                    console.print("[dim]No learning rules defined.[/dim]")
                else:
                    console.print(f"[dim]No {status} rules found.[/dim]")
                return 0

            console.print("\n[bold]Learning Rules[/bold]")
            console.print("=" * 60)
            console.print()

            table = Table(show_header=True, header_style="bold")
            table.add_column("ID", justify="right", style="dim", width=4)
            table.add_column("Type", width=16)
            table.add_column("Match", width=22)
            table.add_column("Action", width=19)
            table.add_column("Status", width=8)
            table.add_column("Effectiveness", justify="right", width=13)

            for rule in rules:
                # Format rule type
                type_str = rule.rule_type.value.replace("_", " ").title()
                if len(type_str) > 16:
                    type_str = type_str[:13] + "..."

                # Format match criteria
                match_str = ""
                if "sender" in rule.match_criteria:
                    match_str = rule.match_criteria["sender"]
                elif "pattern" in rule.match_criteria:
                    match_str = f'"{rule.match_criteria["pattern"]}"'
                else:
                    match_str = str(rule.match_criteria)[:20]
                if len(match_str) > 22:
                    match_str = match_str[:19] + "..."

                # Format action
                action_str = ""
                if "category" in rule.action:
                    action_str = f"category: {rule.action['category']}"
                else:
                    action_str = str(rule.action)[:17]
                if len(action_str) > 19:
                    action_str = action_str[:16] + "..."

                # Format status with color
                status_color = "green" if rule.status == LearningRuleStatus.ACTIVE else "dim"
                status_str = rule.status.value

                # Format effectiveness
                if rule.effectiveness_score:
                    eff_pct = float(rule.effectiveness_score) * 100
                    eff_str = f"{eff_pct:.0f}%"
                else:
                    eff_str = "-"

                table.add_row(
                    str(rule.id),
                    type_str,
                    match_str,
                    action_str,
                    f"[{status_color}]{status_str}[/{status_color}]",
                    eff_str,
                )

            console.print(table)
            return 0

        except Exception as e:
            console.print(f"\n[red]Unexpected error:[/red] {e}")
            return 1

        finally:
            from penf_lib.storage.connections import cleanup_connections
            await cleanup_connections()

    result = asyncio.run(_list_async())
    sys.exit(result)


@rules_group.command("show")
@click.argument("rule_id", type=int)
@click.pass_context
def rules_show(ctx: click.Context, rule_id: int) -> None:
    """Show detailed information about a specific rule.

    Displays match criteria, action, status, thresholds,
    and effectiveness statistics.

    Examples:
        penf review rules show 1   # Show rule #1
    """
    async def _show_async() -> int:
        try:
            rule_manager = get_mock_rule_manager()
            rule = await rule_manager.get_rule_by_id(rule_id)

            if not rule:
                console.print(f"[red]Rule #{rule_id} not found.[/red]")
                return 1

            # Build display
            type_title = rule.rule_type.value.replace("_", " ").title()
            console.print(f"\n[bold]Rule #{rule.id}: {type_title}[/bold]")
            console.print("=" * 40)

            # Match criteria section
            console.print("\n[bold]Match Criteria:[/bold]")
            for key, value in rule.match_criteria.items():
                console.print(f"  {key}: {value}")

            # Action section
            console.print("\n[bold]Action:[/bold]")
            for key, value in rule.action.items():
                console.print(f"  {key}: {value}")

            # Status and thresholds
            status_color = "green" if rule.status == LearningRuleStatus.ACTIVE else "yellow"
            console.print(f"\n[bold]Status:[/bold] [{status_color}]{rule.status.value}[/{status_color}]")
            console.print(f"[bold]Priority:[/bold] {rule.priority}")
            console.print(f"[bold]Confidence Threshold:[/bold] {rule.confidence_threshold:.2f}")

            # Effectiveness section
            console.print("\n[bold]Effectiveness:[/bold]")
            console.print(f"  Times Applied: {rule.times_applied}")
            console.print(f"  Times Overridden: {rule.times_overridden}")
            if rule.effectiveness_score:
                eff_pct = float(rule.effectiveness_score) * 100
                console.print(f"  Score: {eff_pct:.1f}%")
            else:
                console.print("  Score: [dim]not enough data[/dim]")

            # Timestamps
            console.print(f"\n[bold]Created:[/bold] {rule.created_at.strftime('%Y-%m-%d %H:%M')}")
            if rule.last_applied_at:
                console.print(f"[bold]Last Applied:[/bold] {rule.last_applied_at.strftime('%Y-%m-%d %H:%M')}")

            if rule.description:
                console.print(f"\n[dim]{rule.description}[/dim]")

            return 0

        except Exception as e:
            console.print(f"\n[red]Unexpected error:[/red] {e}")
            return 1

        finally:
            from penf_lib.storage.connections import cleanup_connections
            await cleanup_connections()

    result = asyncio.run(_show_async())
    sys.exit(result)


@rules_group.command("enable")
@click.argument("rule_id", type=int)
@click.pass_context
def rules_enable(ctx: click.Context, rule_id: int) -> None:
    """Enable a disabled rule.

    Reactivates a previously disabled learning rule so it will
    be applied to future content processing.

    Examples:
        penf review rules enable 3   # Enable rule #3
    """
    async def _enable_async() -> int:
        try:
            rule_manager = get_mock_rule_manager()
            rule = await rule_manager.get_rule_by_id(rule_id)

            if not rule:
                console.print(f"[red]Rule #{rule_id} not found.[/red]")
                return 1

            if rule.status == LearningRuleStatus.ACTIVE:
                console.print(f"[yellow]Rule #{rule_id} is already active.[/yellow]")
                return 0

            updated = await rule_manager.enable_rule(rule_id)
            if updated:
                console.print(f"[green]Rule #{rule_id} enabled.[/green]")
                type_str = updated.rule_type.value.replace("_", " ").title()
                console.print(f"[dim]{type_str}: {updated.name}[/dim]")
                return 0
            else:
                console.print(f"[red]Failed to enable rule #{rule_id}.[/red]")
                return 1

        except Exception as e:
            console.print(f"\n[red]Unexpected error:[/red] {e}")
            return 1

        finally:
            from penf_lib.storage.connections import cleanup_connections
            await cleanup_connections()

    result = asyncio.run(_enable_async())
    sys.exit(result)


@rules_group.command("disable")
@click.argument("rule_id", type=int)
@click.pass_context
def rules_disable(ctx: click.Context, rule_id: int) -> None:
    """Disable an active rule.

    Deactivates a learning rule so it will no longer be applied
    to future content processing.

    Examples:
        penf review rules disable 2   # Disable rule #2
    """
    async def _disable_async() -> int:
        try:
            rule_manager = get_mock_rule_manager()
            rule = await rule_manager.get_rule_by_id(rule_id)

            if not rule:
                console.print(f"[red]Rule #{rule_id} not found.[/red]")
                return 1

            if rule.status == LearningRuleStatus.DISABLED:
                console.print(f"[yellow]Rule #{rule_id} is already disabled.[/yellow]")
                return 0

            updated = await rule_manager.disable_rule(rule_id)
            if updated:
                console.print(f"[yellow]Rule #{rule_id} disabled.[/yellow]")
                type_str = updated.rule_type.value.replace("_", " ").title()
                console.print(f"[dim]{type_str}: {updated.name}[/dim]")
                return 0
            else:
                console.print(f"[red]Failed to disable rule #{rule_id}.[/red]")
                return 1

        except Exception as e:
            console.print(f"\n[red]Unexpected error:[/red] {e}")
            return 1

        finally:
            from penf_lib.storage.connections import cleanup_connections
            await cleanup_connections()

    result = asyncio.run(_disable_async())
    sys.exit(result)


@rules_group.command("pending")
@click.pass_context
def rules_pending(ctx: click.Context) -> None:
    """View pending rule suggestions.

    Shows rule patterns detected from your review decisions that
    could be converted into automated learning rules.

    Examples:
        penf review rules pending   # View pending suggestions
    """
    from rich.table import Table

    async def _pending_async() -> int:
        try:
            rule_manager = get_mock_rule_manager()
            suggestions = await rule_manager.get_pending_suggestions()

            if not suggestions:
                console.print("[dim]No pending rule suggestions.[/dim]")
                console.print("\n[dim]Suggestions are generated as you make review decisions.[/dim]")
                return 0

            console.print("\n[bold]Pending Rule Suggestions[/bold]")
            console.print("=" * 60)
            console.print()

            table = Table(show_header=True, header_style="bold")
            table.add_column("ID", justify="right", style="dim", width=4)
            table.add_column("Type", width=16)
            table.add_column("Pattern", width=23)
            table.add_column("Confidence", justify="right", width=10)
            table.add_column("Occurrences", justify="right", width=11)

            for suggestion in suggestions:
                # Format type
                type_str = suggestion.rule_type.value.replace("_", " ").title()
                if len(type_str) > 16:
                    type_str = type_str[:13] + "..."

                # Format pattern
                pattern_str = suggestion.pattern
                if len(pattern_str) > 23:
                    pattern_str = pattern_str[:20] + "..."

                # Format confidence
                conf_str = f"{suggestion.confidence:.2f}"

                table.add_row(
                    str(suggestion.id),
                    type_str,
                    pattern_str,
                    conf_str,
                    str(suggestion.occurrences),
                )

            console.print(table)
            console.print()
            console.print("[dim]Accept with: penf review rules accept <ID>[/dim]")
            console.print("[dim]Reject with: penf review rules reject <ID>[/dim]")

            return 0

        except Exception as e:
            console.print(f"\n[red]Unexpected error:[/red] {e}")
            return 1

        finally:
            from penf_lib.storage.connections import cleanup_connections
            await cleanup_connections()

    result = asyncio.run(_pending_async())
    sys.exit(result)


@rules_group.command("accept")
@click.argument("suggestion_id", type=int)
@click.pass_context
def rules_accept(ctx: click.Context, suggestion_id: int) -> None:
    """Accept a pending rule suggestion.

    Converts a pending suggestion into an active learning rule
    that will be applied to future content processing.

    Examples:
        penf review rules accept 101   # Accept suggestion #101
    """
    async def _accept_async() -> int:
        try:
            rule_manager = get_mock_rule_manager()
            suggestion = await rule_manager.get_suggestion_by_id(suggestion_id)

            if not suggestion:
                console.print(f"[red]Suggestion #{suggestion_id} not found.[/red]")
                return 1

            # Accept and create rule
            rule = await rule_manager.accept_suggestion(suggestion_id)

            if rule:
                console.print(f"[green]Suggestion #{suggestion_id} accepted.[/green]")
                console.print(f"[dim]Created rule #{rule.id}: {rule.name}[/dim]")
                return 0
            else:
                console.print(f"[red]Failed to accept suggestion #{suggestion_id}.[/red]")
                return 1

        except Exception as e:
            console.print(f"\n[red]Unexpected error:[/red] {e}")
            return 1

        finally:
            from penf_lib.storage.connections import cleanup_connections
            await cleanup_connections()

    result = asyncio.run(_accept_async())
    sys.exit(result)


@rules_group.command("reject")
@click.argument("suggestion_id", type=int)
@click.pass_context
def rules_reject(ctx: click.Context, suggestion_id: int) -> None:
    """Reject a pending rule suggestion.

    Removes a pending suggestion without creating a rule.
    The pattern may be suggested again if similar patterns continue.

    Examples:
        penf review rules reject 102   # Reject suggestion #102
    """
    async def _reject_async() -> int:
        try:
            rule_manager = get_mock_rule_manager()
            suggestion = await rule_manager.get_suggestion_by_id(suggestion_id)

            if not suggestion:
                console.print(f"[red]Suggestion #{suggestion_id} not found.[/red]")
                return 1

            # Reject suggestion
            success = await rule_manager.reject_suggestion(suggestion_id)

            if success:
                console.print(f"[yellow]Suggestion #{suggestion_id} rejected.[/yellow]")
                console.print(f"[dim]Pattern: {suggestion.pattern}[/dim]")
                return 0
            else:
                console.print(f"[red]Failed to reject suggestion #{suggestion_id}.[/red]")
                return 1

        except Exception as e:
            console.print(f"\n[red]Unexpected error:[/red] {e}")
            return 1

        finally:
            from penf_lib.storage.connections import cleanup_connections
            await cleanup_connections()

    result = asyncio.run(_reject_async())
    sys.exit(result)
