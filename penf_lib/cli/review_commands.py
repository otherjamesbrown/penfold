"""Click command group for the daily review workflow.

This module implements the `penf review` command group for User Story 1 - Queue Management.
It provides commands for starting, resuming, and managing review sessions.

Commands:
    - penf review: Start or resume a review session
    - penf review status: Show session status
    - penf review queue: Display the review queue
    - penf review next: Show next item in queue
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
    PriorityMode,
    ReviewItemDTO,
    ReviewItemStatus,
    ReviewMode,
    SessionDTO,
    SessionStatus,
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
            console.print(f"\n[yellow]Active session already exists.[/yellow]")
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
            queue_manager = QueueManager(repository)

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
