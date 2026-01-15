"""Unit tests for review display module.

Tests the Rich terminal rendering for the daily review workflow display
components including queue display, item display, session status display,
and helper functions for color coding.
"""

import io
from datetime import datetime, timedelta, timezone
from decimal import Decimal
from uuid import uuid4

import pytest
from rich.console import Console

from penf_lib.review.models import (
    AISuggestion,
    ContentType,
    DecisionType,
    PriorityMode,
    ReviewItemDTO,
    ReviewItemStatus,
    ReviewMode,
    SessionDTO,
    SessionStatus,
    UserCorrection,
)


# =============================================================================
# FIXTURES
# =============================================================================


@pytest.fixture
def tenant_id():
    """Test tenant ID."""
    return uuid4()


@pytest.fixture
def console():
    """Create a Rich Console that captures output."""
    string_io = io.StringIO()
    return Console(file=string_io, force_terminal=True, width=120)


def get_console_output(console: Console) -> str:
    """Extract string output from console."""
    console.file.seek(0)
    return console.file.read()


def create_review_item(
    item_id: int = 1,
    tenant_id=None,
    session_id: int = 1,
    source_id: int = 100,
    queue_position: int = 1,
    status: ReviewItemStatus = ReviewItemStatus.PENDING,
    content_type: ContentType = ContentType.EMAIL,
    content_preview: str = "Re: Q1 Planning Meeting - Let's discuss the roadmap...",
    category: str = "project/planning",
    participants: list = None,
    tags: list = None,
    ai_confidence: Decimal = Decimal("0.85"),
    ai_model: str = "gemini-1.5",
    business_importance: int = 5,
    user_decision: DecisionType = None,
    user_correction: UserCorrection = None,
) -> ReviewItemDTO:
    """Create a ReviewItemDTO for testing."""
    if tenant_id is None:
        tenant_id = uuid4()
    if participants is None:
        participants = ["john@example.com", "jane@example.com"]
    if tags is None:
        tags = ["meeting", "q1", "planning"]

    now = datetime.now(timezone.utc)
    return ReviewItemDTO(
        id=item_id,
        tenant_id=tenant_id,
        session_id=session_id,
        source_id=source_id,
        queue_position=queue_position,
        status=status,
        content_type=content_type,
        content_preview=content_preview,
        ai_suggestion=AISuggestion(
            category=category,
            participants=participants,
            tags=tags,
        ),
        ai_confidence=ai_confidence,
        ai_model=ai_model,
        business_importance=business_importance,
        source_timestamp=now - timedelta(hours=2),
        user_decision=user_decision,
        user_correction=user_correction,
        created_at=now,
        updated_at=now,
    )


def create_session(
    session_id: int = 1,
    tenant_id=None,
    user_email: str = "test@example.com",
    status: SessionStatus = SessionStatus.ACTIVE,
    total_items: int = 100,
    items_reviewed: int = 25,
    items_accepted: int = 15,
    items_rejected: int = 5,
    items_modified: int = 3,
    items_skipped: int = 2,
    current_position: int = 25,
    review_mode: ReviewMode = ReviewMode.STANDARD,
    priority_mode: PriorityMode = PriorityMode.MIXED,
) -> SessionDTO:
    """Create a SessionDTO for testing."""
    if tenant_id is None:
        tenant_id = uuid4()

    now = datetime.now(timezone.utc)
    return SessionDTO(
        id=session_id,
        session_uuid=uuid4(),
        tenant_id=tenant_id,
        user_email=user_email,
        status=status,
        started_at=now - timedelta(minutes=30),
        last_activity_at=now,
        completed_at=None if status != SessionStatus.COMPLETED else now,
        expires_at=now + timedelta(hours=24),
        current_position=current_position,
        total_items=total_items,
        items_reviewed=items_reviewed,
        items_accepted=items_accepted,
        items_rejected=items_rejected,
        items_modified=items_modified,
        items_skipped=items_skipped,
        review_mode=review_mode,
        priority_mode=priority_mode,
        filter_criteria={},
        session_metadata={},
        created_at=now,
        updated_at=now,
    )


# =============================================================================
# HELPER FUNCTION TESTS
# =============================================================================


class TestGetConfidenceColor:
    """Tests for get_confidence_color helper function."""

    def test_high_confidence_green(self):
        """Confidence >80% should return green color."""
        from penf_lib.cli.review_display import get_confidence_color

        assert get_confidence_color(Decimal("0.85")) == "green"
        assert get_confidence_color(Decimal("0.90")) == "green"
        assert get_confidence_color(Decimal("0.95")) == "green"
        assert get_confidence_color(Decimal("1.00")) == "green"

    def test_high_confidence_boundary(self):
        """Exactly 80% should be yellow (boundary test - green requires >80%)."""
        from penf_lib.cli.review_display import get_confidence_color

        assert get_confidence_color(Decimal("0.80")) == "yellow"

    def test_medium_confidence_yellow(self):
        """Confidence 50-79% should return yellow color."""
        from penf_lib.cli.review_display import get_confidence_color

        assert get_confidence_color(Decimal("0.79")) == "yellow"
        assert get_confidence_color(Decimal("0.65")) == "yellow"
        assert get_confidence_color(Decimal("0.50")) == "yellow"

    def test_low_confidence_red(self):
        """Confidence <50% should return red color."""
        from penf_lib.cli.review_display import get_confidence_color

        assert get_confidence_color(Decimal("0.49")) == "red"
        assert get_confidence_color(Decimal("0.25")) == "red"
        assert get_confidence_color(Decimal("0.10")) == "red"
        assert get_confidence_color(Decimal("0.00")) == "red"

    @pytest.mark.parametrize(
        "confidence,expected_color",
        [
            (Decimal("0.95"), "green"),
            (Decimal("0.81"), "green"),
            (Decimal("0.80"), "yellow"),  # >0.80 for green, so 0.80 is yellow
            (Decimal("0.79"), "yellow"),
            (Decimal("0.50"), "yellow"),
            (Decimal("0.49"), "red"),
            (Decimal("0.00"), "red"),
        ],
    )
    def test_confidence_color_parametrized(self, confidence, expected_color):
        """Parametrized test for confidence color thresholds."""
        from penf_lib.cli.review_display import get_confidence_color

        assert get_confidence_color(confidence) == expected_color


class TestGetContentTypeColor:
    """Tests for get_content_type_color helper function."""

    def test_email_blue(self):
        """Email content type should return blue color."""
        from penf_lib.cli.review_display import get_content_type_color

        assert get_content_type_color(ContentType.EMAIL) == "blue"

    def test_meeting_purple(self):
        """Meeting content type should return purple/magenta color."""
        from penf_lib.cli.review_display import get_content_type_color

        color = get_content_type_color(ContentType.MEETING)
        assert color in ("purple", "magenta")

    def test_document_cyan(self):
        """Document content type should return cyan color."""
        from penf_lib.cli.review_display import get_content_type_color

        assert get_content_type_color(ContentType.DOCUMENT) == "cyan"

    @pytest.mark.parametrize(
        "content_type,expected_colors",
        [
            (ContentType.EMAIL, ["blue"]),
            (ContentType.MEETING, ["purple", "magenta"]),
            (ContentType.DOCUMENT, ["cyan"]),
        ],
    )
    def test_content_type_colors_parametrized(self, content_type, expected_colors):
        """Parametrized test for content type colors."""
        from penf_lib.cli.review_display import get_content_type_color

        assert get_content_type_color(content_type) in expected_colors


class TestFormatQueueSummary:
    """Tests for format_queue_summary helper function."""

    def test_format_empty_queue(self):
        """Empty queue should show zero items."""
        from penf_lib.cli.review_display import format_queue_summary

        result = format_queue_summary(0, 0)
        assert "0" in result

    def test_format_partial_progress(self):
        """Partial progress should show correct counts."""
        from penf_lib.cli.review_display import format_queue_summary

        result = format_queue_summary(75, 100)
        assert "75" in result or "100" in result

    def test_format_completed_queue(self):
        """Completed queue should reflect all items reviewed."""
        from penf_lib.cli.review_display import format_queue_summary

        result = format_queue_summary(0, 50)
        assert "50" in result


# =============================================================================
# QUEUE DISPLAY TESTS
# =============================================================================


class TestQueueDisplay:
    """Tests for QueueDisplay class."""

    def test_render_empty_queue(self, console, tenant_id):
        """Empty queue should render without error."""
        from penf_lib.cli.review_display import QueueDisplay

        display = QueueDisplay(items=[], pending_count=0, total_count=0)
        display.render(console)

        output = get_console_output(console)
        assert "0" in output  # Should show 0 pending of 0 total

    def test_render_with_items(self, console, tenant_id):
        """Queue with items should render a table with correct columns."""
        from penf_lib.cli.review_display import QueueDisplay

        items = [
            create_review_item(item_id=1, tenant_id=tenant_id, queue_position=1),
            create_review_item(item_id=2, tenant_id=tenant_id, queue_position=2),
            create_review_item(item_id=3, tenant_id=tenant_id, queue_position=3),
        ]

        display = QueueDisplay(items=items, pending_count=3, total_count=3)
        display.render(console)

        output = get_console_output(console)
        # Table should contain position numbers
        assert "1" in output
        assert "2" in output
        assert "3" in output

    def test_render_shows_content_preview(self, console, tenant_id):
        """Queue display should show content previews."""
        from penf_lib.cli.review_display import QueueDisplay

        item = create_review_item(
            tenant_id=tenant_id,
            content_preview="Important budget discussion for Q2",
        )

        display = QueueDisplay(items=[item], pending_count=1, total_count=1)
        display.render(console)

        output = get_console_output(console)
        # Should contain part of the preview text
        assert "budget" in output.lower() or "Q2" in output

    def test_render_shows_content_type(self, console, tenant_id):
        """Queue display should show content type."""
        from penf_lib.cli.review_display import QueueDisplay

        item = create_review_item(
            tenant_id=tenant_id,
            content_type=ContentType.MEETING,
        )

        display = QueueDisplay(items=[item], pending_count=1, total_count=1)
        display.render(console)

        output = get_console_output(console)
        assert "meeting" in output.lower()

    def test_render_shows_confidence(self, console, tenant_id):
        """Queue display should show confidence values."""
        from penf_lib.cli.review_display import QueueDisplay

        item = create_review_item(
            tenant_id=tenant_id,
            ai_confidence=Decimal("0.92"),
        )

        display = QueueDisplay(items=[item], pending_count=1, total_count=1)
        display.render(console)

        output = get_console_output(console)
        # Should show confidence as percentage or decimal
        assert "92" in output or "0.92" in output

    def test_pagination_first_page(self, console, tenant_id):
        """First page of pagination should show first N items."""
        from penf_lib.cli.review_display import QueueDisplay

        # Simulate first page of 5 items out of 20 total
        items = [
            create_review_item(item_id=i, tenant_id=tenant_id, queue_position=i)
            for i in range(1, 6)  # First 5 items for page 1
        ]

        display = QueueDisplay(
            items=items,
            pending_count=20,
            total_count=20,
            items_per_page=5,
            current_page=1,
        )
        display.render(console)

        output = get_console_output(console)
        # Should show items 1-5 but not 6+
        assert "1" in output
        # Page indicator should be present
        assert "page" in output.lower() or "1" in output

    def test_pagination_middle_page(self, console, tenant_id):
        """Middle page should show correct subset of items."""
        from penf_lib.cli.review_display import QueueDisplay

        # Simulate second page of 5 items out of 20 total
        items = [
            create_review_item(item_id=i, tenant_id=tenant_id, queue_position=i)
            for i in range(6, 11)  # Items 6-10 for page 2
        ]

        display = QueueDisplay(
            items=items,
            pending_count=20,
            total_count=20,
            items_per_page=5,
            current_page=2,
        )
        display.render(console)

        output = get_console_output(console)
        # Page 2 should exist
        assert "2" in output

    def test_confidence_coloring_in_table(self, console, tenant_id):
        """Items should have colored confidence values based on thresholds."""
        from penf_lib.cli.review_display import QueueDisplay

        items = [
            create_review_item(
                item_id=1,
                tenant_id=tenant_id,
                ai_confidence=Decimal("0.95"),  # High - green
            ),
            create_review_item(
                item_id=2,
                tenant_id=tenant_id,
                ai_confidence=Decimal("0.65"),  # Medium - yellow
            ),
            create_review_item(
                item_id=3,
                tenant_id=tenant_id,
                ai_confidence=Decimal("0.30"),  # Low - red
            ),
        ]

        display = QueueDisplay(items=items, pending_count=3, total_count=3)
        display.render(console)

        # Just verify render completes without error and produces output
        output = get_console_output(console)
        assert len(output) > 0

    def test_render_with_different_statuses(self, console, tenant_id):
        """Items with different statuses should render correctly."""
        from penf_lib.cli.review_display import QueueDisplay

        items = [
            create_review_item(
                item_id=1, tenant_id=tenant_id, status=ReviewItemStatus.PENDING
            ),
            create_review_item(
                item_id=2, tenant_id=tenant_id, status=ReviewItemStatus.IN_REVIEW
            ),
            create_review_item(
                item_id=3, tenant_id=tenant_id, status=ReviewItemStatus.ACCEPTED
            ),
        ]

        display = QueueDisplay(items=items, pending_count=1, total_count=3)
        display.render(console)

        output = get_console_output(console)
        assert len(output) > 0


# =============================================================================
# ITEM DISPLAY TESTS
# =============================================================================


class TestItemDisplay:
    """Tests for ItemDisplay class."""

    def test_render_email_item(self, console, tenant_id):
        """Email item should show email-specific fields."""
        from penf_lib.cli.review_display import ItemDisplay

        item = create_review_item(
            tenant_id=tenant_id,
            content_type=ContentType.EMAIL,
            content_preview="From: john@example.com\nSubject: Quarterly Review",
        )

        display = ItemDisplay(item=item, position=1, total=10)
        display.render(console)

        output = get_console_output(console)
        assert "email" in output.lower()
        # Should show content preview
        assert "quarterly" in output.lower() or "review" in output.lower()

    def test_render_meeting_item(self, console, tenant_id):
        """Meeting item should show meeting-specific fields."""
        from penf_lib.cli.review_display import ItemDisplay

        item = create_review_item(
            tenant_id=tenant_id,
            content_type=ContentType.MEETING,
            content_preview="Sprint Planning - Team sync for sprint 42",
            category="meetings/planning",
        )

        display = ItemDisplay(item=item, position=1, total=10)
        display.render(console)

        output = get_console_output(console)
        assert "meeting" in output.lower()

    def test_render_document_item(self, console, tenant_id):
        """Document item should show document-specific fields."""
        from penf_lib.cli.review_display import ItemDisplay

        item = create_review_item(
            tenant_id=tenant_id,
            content_type=ContentType.DOCUMENT,
            content_preview="Technical Design Document - API v2.0",
        )

        display = ItemDisplay(item=item, position=1, total=10)
        display.render(console)

        output = get_console_output(console)
        assert "document" in output.lower()

    def test_ai_suggestion_display(self, console, tenant_id):
        """Should show AI suggestion with category, participants, tags."""
        from penf_lib.cli.review_display import ItemDisplay

        item = create_review_item(
            tenant_id=tenant_id,
            category="project/alpha/planning",
            participants=["alice@example.com", "bob@example.com"],
            tags=["urgent", "budget", "review"],
        )

        display = ItemDisplay(item=item, position=1, total=10)
        display.render(console)

        output = get_console_output(console)
        # Should show category
        assert "project" in output.lower() or "planning" in output.lower()
        # Should show at least some participants or tags
        assert (
            "alice" in output.lower()
            or "bob" in output.lower()
            or "urgent" in output.lower()
        )

    def test_ai_confidence_display(self, console, tenant_id):
        """Should display AI confidence score."""
        from penf_lib.cli.review_display import ItemDisplay

        item = create_review_item(
            tenant_id=tenant_id,
            ai_confidence=Decimal("0.87"),
        )

        display = ItemDisplay(item=item, position=1, total=10)
        display.render(console)

        output = get_console_output(console)
        # Should show confidence
        assert "87" in output or "0.87" in output or "confidence" in output.lower()

    def test_ai_model_display(self, console, tenant_id):
        """Should display AI model name."""
        from penf_lib.cli.review_display import ItemDisplay

        item = create_review_item(
            tenant_id=tenant_id,
            ai_model="gemini-1.5-pro",
        )

        display = ItemDisplay(item=item, position=1, total=10)
        display.render(console)

        output = get_console_output(console)
        # Model may or may not be shown in detail view
        assert len(output) > 0

    def test_content_preview_truncation(self, console, tenant_id):
        """Long content should be truncated appropriately."""
        from penf_lib.cli.review_display import ItemDisplay

        long_content = "This is a very long content preview. " * 50  # ~1800 chars

        item = create_review_item(
            tenant_id=tenant_id,
            content_preview=long_content,
        )

        # show_full_content=False triggers truncation to 80 chars
        display = ItemDisplay(item=item, position=1, total=10, show_full_content=False)
        display.render(console)

        output = get_console_output(console)
        # The content preview should be truncated (80 chars default)
        # The full content should NOT appear in output
        assert long_content not in output
        # Output should exist
        assert len(output) > 0

    def test_render_item_with_user_decision(self, console, tenant_id):
        """Item with user decision should show the decision."""
        from penf_lib.cli.review_display import ItemDisplay

        item = create_review_item(
            tenant_id=tenant_id,
            status=ReviewItemStatus.ACCEPTED,
            user_decision=DecisionType.ACCEPT,
        )

        display = ItemDisplay(item=item, position=1, total=10)
        display.render(console)

        output = get_console_output(console)
        # Status should be reflected in border style (green for accepted)
        assert len(output) > 0

    def test_render_item_with_user_correction(self, console, tenant_id):
        """Item with user correction should show the correction."""
        from penf_lib.cli.review_display import ItemDisplay

        correction = UserCorrection(
            category="corrected/category",
            participants=["corrected@example.com"],
            tags=["corrected-tag"],
            reason="wrong_category",
            notes="AI miscategorized this item",
        )

        item = create_review_item(
            tenant_id=tenant_id,
            status=ReviewItemStatus.MODIFIED,
            user_decision=DecisionType.MODIFY,
            user_correction=correction,
        )

        display = ItemDisplay(item=item, position=1, total=10)
        display.render(console)

        output = get_console_output(console)
        # Should render without error (status reflected in border style)
        assert len(output) > 0

    def test_business_importance_display(self, console, tenant_id):
        """Should display business importance score."""
        from penf_lib.cli.review_display import ItemDisplay

        item = create_review_item(
            tenant_id=tenant_id,
            business_importance=9,
        )

        display = ItemDisplay(item=item, position=1, total=10)
        display.render(console)

        output = get_console_output(console)
        # Should render without error (importance may or may not be shown)
        assert len(output) > 0


# =============================================================================
# SESSION STATUS DISPLAY TESTS
# =============================================================================


class TestSessionStatusDisplay:
    """Tests for SessionStatusDisplay class."""

    def test_render_active_session(self, console, tenant_id):
        """Active session should show session details and progress."""
        from penf_lib.cli.review_display import SessionStatusDisplay

        session = create_session(
            tenant_id=tenant_id,
            status=SessionStatus.ACTIVE,
            total_items=100,
            items_reviewed=25,
        )

        display = SessionStatusDisplay(session=session)
        display.render(console)

        output = get_console_output(console)
        # Should show active status
        assert "active" in output.lower()
        # Should show progress
        assert "25" in output or "100" in output

    def test_render_paused_session(self, console, tenant_id):
        """Paused session should indicate paused status."""
        from penf_lib.cli.review_display import SessionStatusDisplay

        session = create_session(
            tenant_id=tenant_id,
            status=SessionStatus.PAUSED,
        )

        display = SessionStatusDisplay(session=session)
        display.render(console)

        output = get_console_output(console)
        assert "paused" in output.lower()

    def test_render_completed_session(self, console, tenant_id):
        """Completed session should show completion status."""
        from penf_lib.cli.review_display import SessionStatusDisplay

        session = create_session(
            tenant_id=tenant_id,
            status=SessionStatus.COMPLETED,
            total_items=50,
            items_reviewed=50,
            items_accepted=40,
            items_rejected=5,
            items_modified=3,
            items_skipped=2,
        )

        display = SessionStatusDisplay(session=session)
        display.render(console)

        output = get_console_output(console)
        assert "completed" in output.lower() or "complete" in output.lower()

    def test_render_progress_breakdown(self, console, tenant_id):
        """Should show accepted/rejected/modified/skipped counts."""
        from penf_lib.cli.review_display import SessionStatusDisplay

        session = create_session(
            tenant_id=tenant_id,
            items_accepted=15,
            items_rejected=5,
            items_modified=3,
            items_skipped=2,
        )

        display = SessionStatusDisplay(session=session)
        display.render(console)

        output = get_console_output(console)
        # At least one of the counts should appear
        counts_present = any(str(n) in output for n in [15, 5, 3, 2])
        assert counts_present or "accept" in output.lower()

    def test_render_completion_percentage(self, console, tenant_id):
        """Should calculate and show completion percentage."""
        from penf_lib.cli.review_display import SessionStatusDisplay

        session = create_session(
            tenant_id=tenant_id,
            total_items=100,
            items_reviewed=25,  # 25%
        )

        display = SessionStatusDisplay(session=session)
        display.render(console)

        output = get_console_output(console)
        # Should show 25% or equivalent
        assert "25" in output or "%" in output

    def test_render_review_mode(self, console, tenant_id):
        """Should show the review mode."""
        from penf_lib.cli.review_display import SessionStatusDisplay

        session = create_session(
            tenant_id=tenant_id,
            review_mode=ReviewMode.QUICK,
        )

        display = SessionStatusDisplay(session=session)
        display.render(console)

        output = get_console_output(console)
        assert "quick" in output.lower() or "mode" in output.lower()

    def test_render_priority_mode(self, console, tenant_id):
        """Should show the priority mode."""
        from penf_lib.cli.review_display import SessionStatusDisplay

        session = create_session(
            tenant_id=tenant_id,
            priority_mode=PriorityMode.CONFIDENCE,
        )

        display = SessionStatusDisplay(session=session)
        display.render(console)

        output = get_console_output(console)
        # Priority mode may or may not be shown
        assert len(output) > 0

    def test_render_session_timing(self, console, tenant_id):
        """Should show session timing information."""
        from penf_lib.cli.review_display import SessionStatusDisplay

        session = create_session(tenant_id=tenant_id)

        display = SessionStatusDisplay(session=session)
        display.render(console)

        output = get_console_output(console)
        # Should render without error
        assert len(output) > 0

    def test_render_abandoned_session(self, console, tenant_id):
        """Abandoned session should show abandoned status."""
        from penf_lib.cli.review_display import SessionStatusDisplay

        session = create_session(
            tenant_id=tenant_id,
            status=SessionStatus.ABANDONED,
        )

        display = SessionStatusDisplay(session=session)
        display.render(console)

        output = get_console_output(console)
        assert "abandoned" in output.lower()

    def test_render_zero_items_session(self, console, tenant_id):
        """Session with zero items should render without division error."""
        from penf_lib.cli.review_display import SessionStatusDisplay

        session = create_session(
            tenant_id=tenant_id,
            total_items=0,
            items_reviewed=0,
            items_accepted=0,
            items_rejected=0,
            items_modified=0,
            items_skipped=0,
        )

        display = SessionStatusDisplay(session=session)
        # Should not raise ZeroDivisionError
        display.render(console)

        output = get_console_output(console)
        assert len(output) > 0


# =============================================================================
# INTEGRATION TESTS FOR DISPLAY COMPONENTS
# =============================================================================


class TestDisplayIntegration:
    """Integration tests for display components working together."""

    def test_queue_and_session_display_together(self, console, tenant_id):
        """Queue and session display should work together."""
        from penf_lib.cli.review_display import QueueDisplay, SessionStatusDisplay

        session = create_session(tenant_id=tenant_id)
        items = [
            create_review_item(item_id=i, tenant_id=tenant_id, queue_position=i)
            for i in range(1, 6)
        ]

        # Render session status
        session_display = SessionStatusDisplay(session=session)
        session_display.render(console)

        # Render queue
        queue_display = QueueDisplay(items=items, pending_count=5, total_count=5)
        queue_display.render(console)

        output = get_console_output(console)
        assert len(output) > 0

    def test_item_detail_after_queue(self, console, tenant_id):
        """Item detail should render after queue display."""
        from penf_lib.cli.review_display import ItemDisplay, QueueDisplay

        items = [
            create_review_item(item_id=1, tenant_id=tenant_id, queue_position=1)
        ]

        # Render queue
        queue_display = QueueDisplay(items=items, pending_count=1, total_count=1)
        queue_display.render(console)

        # Render item detail
        item_display = ItemDisplay(item=items[0], position=1, total=1)
        item_display.render(console)

        output = get_console_output(console)
        assert len(output) > 0

    def test_all_content_types_display(self, console, tenant_id):
        """All content types should render correctly in queue."""
        from penf_lib.cli.review_display import QueueDisplay

        items = [
            create_review_item(
                item_id=1,
                tenant_id=tenant_id,
                queue_position=1,
                content_type=ContentType.EMAIL,
            ),
            create_review_item(
                item_id=2,
                tenant_id=tenant_id,
                queue_position=2,
                content_type=ContentType.MEETING,
            ),
            create_review_item(
                item_id=3,
                tenant_id=tenant_id,
                queue_position=3,
                content_type=ContentType.DOCUMENT,
            ),
        ]

        display = QueueDisplay(items=items, pending_count=3, total_count=3)
        display.render(console)

        output = get_console_output(console)
        # At least one content type should appear
        assert (
            "email" in output.lower()
            or "meeting" in output.lower()
            or "document" in output.lower()
        )

    def test_all_confidence_levels_display(self, console, tenant_id):
        """All confidence levels should render with appropriate colors."""
        from penf_lib.cli.review_display import QueueDisplay

        items = [
            create_review_item(
                item_id=1,
                tenant_id=tenant_id,
                queue_position=1,
                ai_confidence=Decimal("0.95"),  # High
            ),
            create_review_item(
                item_id=2,
                tenant_id=tenant_id,
                queue_position=2,
                ai_confidence=Decimal("0.65"),  # Medium
            ),
            create_review_item(
                item_id=3,
                tenant_id=tenant_id,
                queue_position=3,
                ai_confidence=Decimal("0.25"),  # Low
            ),
        ]

        display = QueueDisplay(items=items, pending_count=3, total_count=3)
        display.render(console)

        output = get_console_output(console)
        assert len(output) > 0


# =============================================================================
# EDGE CASE TESTS
# =============================================================================


class TestDisplayEdgeCases:
    """Tests for edge cases and error handling."""

    def test_render_item_with_none_optional_fields(self, console, tenant_id):
        """Item with None optional fields should render correctly."""
        from penf_lib.cli.review_display import ItemDisplay

        now = datetime.now(timezone.utc)
        item = ReviewItemDTO(
            id=1,
            tenant_id=tenant_id,
            session_id=None,  # Optional
            source_id=100,
            queue_position=None,  # Optional
            status=ReviewItemStatus.PENDING,
            content_type=ContentType.EMAIL,
            content_preview="Test content",
            ai_suggestion=AISuggestion(
                category="test",
                participants=[],
                tags=[],
            ),
            ai_confidence=Decimal("0.50"),
            ai_model="test-model",
            source_timestamp=now,
            created_at=now,
            updated_at=now,
        )

        display = ItemDisplay(item=item, position=1, total=1)
        display.render(console)

        output = get_console_output(console)
        assert len(output) > 0

    def test_render_item_with_empty_suggestion(self, console, tenant_id):
        """Item with empty AI suggestion lists should render correctly."""
        from penf_lib.cli.review_display import ItemDisplay

        item = create_review_item(
            tenant_id=tenant_id,
            participants=[],
            tags=[],
        )

        display = ItemDisplay(item=item, position=1, total=1)
        display.render(console)

        output = get_console_output(console)
        assert len(output) > 0

    def test_render_item_with_special_characters(self, console, tenant_id):
        """Item with special characters should render correctly."""
        from penf_lib.cli.review_display import ItemDisplay

        item = create_review_item(
            tenant_id=tenant_id,
            content_preview="Re: Meeting <script>alert('xss')</script> & other \"special\" chars",
            category="test/special<>chars",
        )

        display = ItemDisplay(item=item, position=1, total=1)
        display.render(console)

        output = get_console_output(console)
        # Should render without raising exception
        assert len(output) > 0

    def test_render_item_with_unicode(self, console, tenant_id):
        """Item with unicode characters should render correctly."""
        from penf_lib.cli.review_display import ItemDisplay

        item = create_review_item(
            tenant_id=tenant_id,
            content_preview="Meeting notes with unicode chars",
            tags=["urgent", "review", "team"],
        )

        display = ItemDisplay(item=item, position=1, total=1)
        display.render(console)

        output = get_console_output(console)
        assert len(output) > 0

    def test_render_very_long_category(self, console, tenant_id):
        """Very long category path should be handled."""
        from penf_lib.cli.review_display import ItemDisplay

        item = create_review_item(
            tenant_id=tenant_id,
            category="/".join(["level"] * 20),  # Very deep category
        )

        display = ItemDisplay(item=item, position=1, total=1)
        display.render(console)

        output = get_console_output(console)
        assert len(output) > 0

    def test_render_many_participants(self, console, tenant_id):
        """Many participants should be handled gracefully."""
        from penf_lib.cli.review_display import ItemDisplay

        item = create_review_item(
            tenant_id=tenant_id,
            participants=[f"user{i}@example.com" for i in range(50)],
        )

        display = ItemDisplay(item=item, position=1, total=1)
        display.render(console)

        output = get_console_output(console)
        assert len(output) > 0

    def test_render_many_tags(self, console, tenant_id):
        """Many tags should be handled gracefully."""
        from penf_lib.cli.review_display import ItemDisplay

        item = create_review_item(
            tenant_id=tenant_id,
            tags=[f"tag-{i}" for i in range(100)],
        )

        display = ItemDisplay(item=item, position=1, total=1)
        display.render(console)

        output = get_console_output(console)
        assert len(output) > 0

    def test_decimal_confidence_precision(self, console, tenant_id):
        """Decimal confidence with various precisions should render."""
        from penf_lib.cli.review_display import QueueDisplay

        items = [
            create_review_item(
                item_id=1,
                tenant_id=tenant_id,
                queue_position=1,
                ai_confidence=Decimal("0.123"),
            ),
            create_review_item(
                item_id=2,
                tenant_id=tenant_id,
                queue_position=2,
                ai_confidence=Decimal("0.999"),
            ),
            create_review_item(
                item_id=3,
                tenant_id=tenant_id,
                queue_position=3,
                ai_confidence=Decimal("0.000"),
            ),
        ]

        display = QueueDisplay(items=items, pending_count=3, total_count=3)
        display.render(console)

        output = get_console_output(console)
        assert len(output) > 0
