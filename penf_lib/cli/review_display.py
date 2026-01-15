"""Rich terminal rendering module for the daily review workflow.

This module provides Rich-based terminal rendering components for review
sessions and queue items. It implements color-coded displays for confidence
levels, status indicators, and content types.

Based on research.md R6 specification for CLI display requirements.
"""

from __future__ import annotations

from datetime import datetime
from decimal import Decimal

from rich.console import Console, RenderableType
from rich.panel import Panel
from rich.table import Table
from rich.text import Text

from penf_lib.review.models import (
    ContentType,
    ReviewItemDTO,
    ReviewItemStatus,
    SessionDTO,
    SessionStatus,
)

# =============================================================================
# HELPER FUNCTIONS
# =============================================================================


def get_confidence_color(confidence: Decimal) -> str:
    """Return the color name for a given confidence score.

    Args:
        confidence: Confidence score between 0.0 and 1.0

    Returns:
        Color name: 'green' for >80%, 'yellow' for 50-80%, 'red' for <50%
    """
    confidence_float = float(confidence)
    if confidence_float > 0.80:
        return "green"
    elif confidence_float >= 0.50:
        return "yellow"
    else:
        return "red"


def get_content_type_color(content_type: ContentType) -> str:
    """Return the color name for a given content type.

    Args:
        content_type: The type of content being displayed

    Returns:
        Color name: 'blue' for email, 'magenta' for meeting, 'cyan' for document
    """
    color_map = {
        ContentType.EMAIL: "blue",
        ContentType.MEETING: "magenta",
        ContentType.DOCUMENT: "cyan",
    }
    return color_map.get(content_type, "white")


def get_status_style(status: ReviewItemStatus) -> str:
    """Return the Rich style for a given review item status.

    Args:
        status: The review item status

    Returns:
        Rich style string for the status
    """
    style_map = {
        ReviewItemStatus.PENDING: "dim",
        ReviewItemStatus.IN_REVIEW: "bold",
        ReviewItemStatus.ACCEPTED: "green",
        ReviewItemStatus.REJECTED: "red",
        ReviewItemStatus.MODIFIED: "cyan",
        ReviewItemStatus.SKIPPED: "yellow",
    }
    return style_map.get(status, "white")


def format_queue_summary(pending: int, total: int) -> str:
    """Format a queue summary header string.

    Args:
        pending: Number of pending items
        total: Total number of items in queue

    Returns:
        Formatted summary string like 'Review Queue (45 pending of 120 total)'
    """
    return f"Review Queue ({pending} pending of {total} total)"


def _format_datetime(dt: datetime) -> str:
    """Format a datetime for display.

    Args:
        dt: The datetime to format

    Returns:
        Formatted datetime string
    """
    return dt.strftime("%Y-%m-%d %H:%M")


def _format_duration(start: datetime, end: datetime | None = None) -> str:
    """Format a duration for display.

    Args:
        start: Start datetime
        end: End datetime (defaults to now if None)

    Returns:
        Human-readable duration string
    """
    if end is None:
        # Use timezone-aware datetime if start is timezone-aware
        if start.tzinfo is not None:
            from datetime import timezone
            end = datetime.now(timezone.utc)
        else:
            end = datetime.now()

    delta = end - start
    total_seconds = int(delta.total_seconds())

    if total_seconds < 60:
        return f"{total_seconds}s"
    elif total_seconds < 3600:
        minutes = total_seconds // 60
        return f"{minutes}m"
    else:
        hours = total_seconds // 3600
        minutes = (total_seconds % 3600) // 60
        return f"{hours}h {minutes}m"


def _truncate_text(text: str, max_length: int = 50) -> str:
    """Truncate text to a maximum length with ellipsis.

    Args:
        text: Text to truncate
        max_length: Maximum length including ellipsis

    Returns:
        Truncated text with ellipsis if needed
    """
    if len(text) <= max_length:
        return text
    return text[: max_length - 3] + "..."


# =============================================================================
# DISPLAY CLASSES
# =============================================================================


class QueueDisplay:
    """Renders the review queue as a Rich Table.

    Displays a paginated list of review items with color-coded columns
    for type, confidence, and status.
    """

    # Navigation hints shown at bottom of queue display
    NAVIGATION_HINTS = "[j/k] Navigate | [Enter] Review | [f] Filter | [q] Exit"

    def __init__(
        self,
        items: list[ReviewItemDTO],
        pending_count: int,
        total_count: int,
        items_per_page: int = 20,
        current_page: int = 1,
        current_position: int | None = None,
    ) -> None:
        """Initialize the queue display.

        Args:
            items: List of review items to display (current page)
            pending_count: Total number of pending items in queue
            total_count: Total number of items in queue
            items_per_page: Number of items per page
            current_page: Current page number (1-indexed)
            current_position: Currently selected item position (1-indexed)
        """
        self.items = items
        self.pending_count = pending_count
        self.total_count = total_count
        self.items_per_page = items_per_page
        self.current_page = current_page
        self.current_position = current_position

    def _build_table(self) -> Table:
        """Build the Rich Table for the queue.

        Returns:
            Configured Rich Table with queue items
        """
        table = Table(
            title=format_queue_summary(self.pending_count, self.total_count),
            show_header=True,
            header_style="bold",
            expand=True,
        )

        # Define columns
        table.add_column("#", style="dim", width=4, justify="right")
        table.add_column("Type", width=8)
        table.add_column("Subject", min_width=30, max_width=50)
        table.add_column("Confidence", width=12, justify="center")
        table.add_column("Source", width=10)

        # Add rows
        for idx, item in enumerate(self.items):
            row_num = (self.current_page - 1) * self.items_per_page + idx + 1
            is_current = self.current_position == row_num

            # Format row number with selection indicator
            num_text = Text(str(row_num))
            if is_current:
                num_text.stylize("bold reverse")

            # Format content type with color
            type_color = get_content_type_color(item.content_type)
            type_text = Text(item.content_type.value[:6], style=type_color)

            # Format subject with status style
            status_style = get_status_style(item.status)
            subject_text = Text(
                _truncate_text(item.content_preview, 45), style=status_style
            )
            if is_current:
                subject_text.stylize("bold")

            # Format confidence with color
            conf_color = get_confidence_color(item.ai_confidence)
            conf_value = f"{float(item.ai_confidence):.2f}"
            conf_text = Text(conf_value, style=conf_color)

            # Source (extract from metadata or use placeholder)
            source_text = Text("gmail", style="dim")  # Default source

            table.add_row(num_text, type_text, subject_text, conf_text, source_text)

        return table

    def get_table(self) -> Table:
        """Get the Rich Table renderable for composition.

        Returns:
            Rich Table containing the queue display
        """
        return self._build_table()

    def render(self, console: Console) -> None:
        """Render the queue display to the console.

        Args:
            console: Rich Console instance for output
        """
        table = self._build_table()
        console.print(table)
        console.print()

        # Pagination info
        total_pages = max(1, (self.total_count + self.items_per_page - 1) // self.items_per_page)
        if total_pages > 1:
            console.print(
                f"[dim]Page {self.current_page} of {total_pages}[/dim]",
                justify="center",
            )
            console.print()

        # Navigation hints
        console.print(f"[dim]{self.NAVIGATION_HINTS}[/dim]", justify="center")


class ItemDisplay:
    """Renders a single review item detail view.

    Displays comprehensive item information in a Rich Panel with
    content preview, AI suggestions, and action hints.
    """

    # Action hints shown at bottom of item display
    ACTION_HINTS = "[a] Accept | [r] Reject | [m] Modify | [s] Skip | [d] Details | [?] Help"

    def __init__(
        self,
        item: ReviewItemDTO,
        position: int,
        total: int,
        show_full_content: bool = False,
    ) -> None:
        """Initialize the item display.

        Args:
            item: Review item to display
            position: Current position in queue (1-indexed)
            total: Total items in queue
            show_full_content: Whether to show full content or preview
        """
        self.item = item
        self.position = position
        self.total = total
        self.show_full_content = show_full_content

    def _build_content(self) -> Text:
        """Build the content for the panel.

        Returns:
            Rich Text object with formatted item content
        """
        text = Text()

        # Header with type and position
        type_color = get_content_type_color(self.item.content_type)
        text.append(f"[{self.item.content_type.value.upper()}]", style=f"bold {type_color}")
        text.append(f"  Item {self.position} of {self.total}\n\n", style="dim")

        # Subject/Preview
        text.append("Subject: ", style="bold")
        content_text = (
            self.item.content_preview
            if self.show_full_content
            else _truncate_text(self.item.content_preview, 80)
        )
        text.append(f"{content_text}\n", style="white")

        # Date and source
        text.append("Date: ", style="bold")
        text.append(f"{_format_datetime(self.item.source_timestamp)}\n", style="dim")

        text.append("Source: ", style="bold")
        text.append("gmail\n\n", style="dim")  # Default source

        # AI Suggestion section
        text.append("AI Suggestion\n", style="bold underline")

        text.append("  Category: ", style="bold")
        text.append(f"{self.item.ai_suggestion.category}\n", style="cyan")

        if self.item.ai_suggestion.participants:
            text.append("  Participants: ", style="bold")
            participants = ", ".join(self.item.ai_suggestion.participants)
            text.append(f"{participants}\n", style="white")

        if self.item.ai_suggestion.tags:
            text.append("  Tags: ", style="bold")
            tags = ", ".join(f"#{tag}" for tag in self.item.ai_suggestion.tags)
            text.append(f"{tags}\n", style="yellow")

        # Confidence with color
        text.append("\n  Confidence: ", style="bold")
        conf_color = get_confidence_color(self.item.ai_confidence)
        conf_value = f"{float(self.item.ai_confidence) * 100:.0f}%"
        text.append(conf_value, style=f"bold {conf_color}")

        # Confidence explanation
        if float(self.item.ai_confidence) < 0.50:
            text.append(" (low - needs review)", style="dim red")
        elif float(self.item.ai_confidence) < 0.80:
            text.append(" (moderate)", style="dim yellow")
        else:
            text.append(" (high)", style="dim green")

        return text

    def get_panel(self) -> Panel:
        """Get the Rich Panel renderable for composition.

        Returns:
            Rich Panel containing the item display
        """
        content = self._build_content()

        # Determine border style based on status
        status_style = get_status_style(self.item.status)

        return Panel(
            content,
            title=f"Review Item #{self.position}",
            subtitle=self.ACTION_HINTS,
            border_style=status_style,
            expand=True,
            padding=(1, 2),
        )

    def render(self, console: Console) -> None:
        """Render the item display to the console.

        Args:
            console: Rich Console instance for output
        """
        panel = self.get_panel()
        console.print(panel)


class SessionStatusDisplay:
    """Renders session status summary.

    Displays session information including progress statistics
    and breakdown by decision type.
    """

    def __init__(
        self,
        session: SessionDTO,
        estimated_time_remaining: int | None = None,
    ) -> None:
        """Initialize the session status display.

        Args:
            session: Session DTO to display
            estimated_time_remaining: Estimated minutes remaining (optional)
        """
        self.session = session
        self.estimated_time_remaining = estimated_time_remaining

    def _build_content(self) -> RenderableType:
        """Build the content for the session status.

        Returns:
            Rich renderable with session status content
        """
        text = Text()

        # Session identity
        text.append("Session ID: ", style="bold")
        text.append(f"{str(self.session.session_uuid)[:8]}...\n", style="dim")

        text.append("Started: ", style="bold")
        text.append(f"{_format_datetime(self.session.started_at)}", style="white")
        text.append(f" ({_format_duration(self.session.started_at)} ago)\n", style="dim")

        # Mode and priority
        text.append("Mode: ", style="bold")
        text.append(f"{self.session.review_mode.value}", style="cyan")
        text.append("  |  ", style="dim")
        text.append("Priority: ", style="bold")
        text.append(f"{self.session.priority_mode.value}\n\n", style="cyan")

        # Progress section
        text.append("Progress\n", style="bold underline")

        reviewed = self.session.items_reviewed
        total = self.session.total_items
        pct = (reviewed / total * 100) if total > 0 else 0

        text.append("  Reviewed: ", style="bold")
        text.append(f"{reviewed} / {total}", style="white")
        text.append(f" ({pct:.0f}%)\n", style="dim")

        # Decision breakdown
        text.append("\n  Breakdown:\n", style="bold")

        # Accepted
        text.append("    Accepted:  ", style="bold")
        text.append(f"{self.session.items_accepted}", style="green")
        text.append("\n")

        # Rejected
        text.append("    Rejected:  ", style="bold")
        text.append(f"{self.session.items_rejected}", style="red")
        text.append("\n")

        # Modified
        text.append("    Modified:  ", style="bold")
        text.append(f"{self.session.items_modified}", style="cyan")
        text.append("\n")

        # Skipped
        text.append("    Skipped:   ", style="bold")
        text.append(f"{self.session.items_skipped}", style="yellow")
        text.append("\n")

        # Estimated time remaining
        if self.estimated_time_remaining is not None:
            text.append("\n")
            text.append("Est. Remaining: ", style="bold")
            if self.estimated_time_remaining < 1:
                text.append("< 1 minute", style="green")
            elif self.estimated_time_remaining == 1:
                text.append("1 minute", style="yellow")
            else:
                text.append(f"{self.estimated_time_remaining} minutes", style="yellow")

        return text

    def _get_status_indicator(self) -> str:
        """Get the status indicator with color.

        Returns:
            Formatted status indicator string
        """
        status_colors = {
            SessionStatus.ACTIVE: "green",
            SessionStatus.PAUSED: "yellow",
            SessionStatus.COMPLETED: "blue",
            SessionStatus.ABANDONED: "red",
        }
        color = status_colors.get(self.session.status, "white")
        return f"[{color}]{self.session.status.value.upper()}[/{color}]"

    def get_panel(self) -> Panel:
        """Get the Rich Panel renderable for composition.

        Returns:
            Rich Panel containing the session status display
        """
        content = self._build_content()
        status_indicator = self._get_status_indicator()

        return Panel(
            content,
            title=f"Review Session - {status_indicator}",
            border_style="blue",
            expand=False,
            padding=(1, 2),
        )

    def render(self, console: Console) -> None:
        """Render the session status to the console.

        Args:
            console: Rich Console instance for output
        """
        panel = self.get_panel()
        console.print(panel)


# =============================================================================
# CONVENIENCE FUNCTIONS
# =============================================================================


def render_queue(
    console: Console,
    items: list[ReviewItemDTO],
    pending_count: int,
    total_count: int,
    items_per_page: int = 20,
    current_page: int = 1,
    current_position: int | None = None,
) -> None:
    """Convenience function to render a queue display.

    Args:
        console: Rich Console instance for output
        items: List of review items to display (current page)
        pending_count: Total number of pending items in queue
        total_count: Total number of items in queue
        items_per_page: Number of items per page
        current_page: Current page number (1-indexed)
        current_position: Currently selected item position (1-indexed)
    """
    display = QueueDisplay(
        items=items,
        pending_count=pending_count,
        total_count=total_count,
        items_per_page=items_per_page,
        current_page=current_page,
        current_position=current_position,
    )
    display.render(console)


def render_item(
    console: Console,
    item: ReviewItemDTO,
    position: int,
    total: int,
    show_full_content: bool = False,
) -> None:
    """Convenience function to render an item display.

    Args:
        console: Rich Console instance for output
        item: Review item to display
        position: Current position in queue (1-indexed)
        total: Total items in queue
        show_full_content: Whether to show full content or preview
    """
    display = ItemDisplay(
        item=item,
        position=position,
        total=total,
        show_full_content=show_full_content,
    )
    display.render(console)


def render_session_status(
    console: Console,
    session: SessionDTO,
    estimated_time_remaining: int | None = None,
) -> None:
    """Convenience function to render session status.

    Args:
        console: Rich Console instance for output
        session: Session DTO to display
        estimated_time_remaining: Estimated minutes remaining (optional)
    """
    display = SessionStatusDisplay(
        session=session,
        estimated_time_remaining=estimated_time_remaining,
    )
    display.render(console)
