"""Unit tests for SessionManager.

Tests the session lifecycle management including creation, state transitions,
progress tracking, and error handling. Uses mocked repository for isolation.
"""

from datetime import datetime, timedelta, timezone
from unittest.mock import AsyncMock, MagicMock
from uuid import uuid4

import pytest

from penf_lib.review.exceptions import (
    ActiveSessionExistsError,
    InvalidSessionStateError,
    PendingItemsError,
    SessionNotFoundError,
)
from penf_lib.review.models import (
    PriorityMode,
    ReviewMode,
    SessionStatus,
)
from penf_lib.review.session import SessionManager


@pytest.fixture
def tenant_id():
    """Test tenant ID."""
    return uuid4()


@pytest.fixture
def user_email():
    """Test user email."""
    return "test@example.com"


@pytest.fixture
def mock_repository():
    """Create mock repository for testing."""
    repo = AsyncMock()
    repo.create_session = AsyncMock()
    repo.get_session_by_id = AsyncMock()
    repo.get_active_session_for_user = AsyncMock()
    repo.update_session = AsyncMock()
    return repo


@pytest.fixture
def session_manager(mock_repository):
    """Create SessionManager with mocked repository."""
    return SessionManager(repository=mock_repository)


def create_mock_session(
    session_id: int = 1,
    tenant_id=None,
    user_email: str = "test@example.com",
    status: SessionStatus = SessionStatus.ACTIVE,
    total_items: int = 100,
    items_reviewed: int = 0,
    items_accepted: int = 0,
    items_rejected: int = 0,
    items_modified: int = 0,
    items_skipped: int = 0,
    current_position: int = 0,
):
    """Create a mock session object for testing."""
    if tenant_id is None:
        tenant_id = uuid4()

    now = datetime.now(timezone.utc)
    session = MagicMock()
    session.id = session_id
    session.session_uuid = uuid4()
    session.tenant_id = tenant_id
    session.user_email = user_email
    session.status = status
    session.started_at = now
    session.last_activity_at = now
    session.completed_at = None
    session.expires_at = now + timedelta(hours=24)
    session.current_position = current_position
    session.total_items = total_items
    session.items_reviewed = items_reviewed
    session.items_accepted = items_accepted
    session.items_rejected = items_rejected
    session.items_modified = items_modified
    session.items_skipped = items_skipped
    session.review_mode = ReviewMode.STANDARD
    session.priority_mode = PriorityMode.MIXED
    session.filter_criteria = {}
    session.session_metadata = {}
    session.created_at = now
    session.updated_at = now
    return session


class TestCreateSession:
    """Tests for SessionManager.create_session()."""

    @pytest.mark.asyncio
    async def test_create_session_success(
        self,
        session_manager: SessionManager,
        mock_repository: AsyncMock,
        tenant_id,
        user_email,
    ):
        """Test creating a new session successfully."""
        # No existing active session
        mock_repository.get_active_session_for_user.return_value = None

        # Repository returns created session with ID
        def create_side_effect(session_data: dict):
            session = create_mock_session(
                session_id=1,
                tenant_id=session_data["tenant_id"],
                user_email=session_data["user_email"],
                total_items=session_data["total_items"],
            )
            session.session_uuid = session_data["session_uuid"]
            return session

        mock_repository.create_session.side_effect = create_side_effect

        result = await session_manager.create_session(
            tenant_id=tenant_id,
            user_email=user_email,
            total_items=50,
            mode=ReviewMode.QUICK,
            priority=PriorityMode.CONFIDENCE,
        )

        assert result.id == 1
        assert result.user_email == user_email
        assert result.total_items == 50
        assert result.status == SessionStatus.ACTIVE
        assert result.items_reviewed == 0

        mock_repository.get_active_session_for_user.assert_called_once_with(
            tenant_id, user_email
        )
        mock_repository.create_session.assert_called_once()

    @pytest.mark.asyncio
    async def test_create_session_with_existing_active_raises(
        self,
        session_manager: SessionManager,
        mock_repository: AsyncMock,
        tenant_id,
        user_email,
    ):
        """Test that creating session raises if active session exists."""
        existing_session = create_mock_session(session_id=42, tenant_id=tenant_id)
        mock_repository.get_active_session_for_user.return_value = existing_session

        with pytest.raises(ActiveSessionExistsError) as exc_info:
            await session_manager.create_session(
                tenant_id=tenant_id,
                user_email=user_email,
                total_items=50,
            )

        assert exc_info.value.code == "REVIEW_002"
        assert exc_info.value.details["existing_session_id"] == 42
        mock_repository.create_session.assert_not_called()


class TestGetActiveSession:
    """Tests for SessionManager.get_active_session()."""

    @pytest.mark.asyncio
    async def test_get_active_session_exists(
        self,
        session_manager: SessionManager,
        mock_repository: AsyncMock,
        tenant_id,
        user_email,
    ):
        """Test getting existing active session."""
        existing = create_mock_session(
            session_id=1,
            tenant_id=tenant_id,
            user_email=user_email,
        )
        mock_repository.get_active_session_for_user.return_value = existing

        result = await session_manager.get_active_session(tenant_id, user_email)

        assert result is not None
        assert result.id == 1
        assert result.user_email == user_email

    @pytest.mark.asyncio
    async def test_get_active_session_none(
        self,
        session_manager: SessionManager,
        mock_repository: AsyncMock,
        tenant_id,
        user_email,
    ):
        """Test getting active session when none exists."""
        mock_repository.get_active_session_for_user.return_value = None

        result = await session_manager.get_active_session(tenant_id, user_email)

        assert result is None


class TestResumeSession:
    """Tests for SessionManager.resume_session()."""

    @pytest.mark.asyncio
    async def test_resume_session_active(
        self,
        session_manager: SessionManager,
        mock_repository: AsyncMock,
    ):
        """Test resuming an active session."""
        session = create_mock_session(
            session_id=1,
            status=SessionStatus.ACTIVE,
        )
        mock_repository.get_session_by_id.return_value = session
        mock_repository.update_session.return_value = session

        result = await session_manager.resume_session(1)

        assert result.status == SessionStatus.ACTIVE
        mock_repository.update_session.assert_called_once()

    @pytest.mark.asyncio
    async def test_resume_session_paused(
        self,
        session_manager: SessionManager,
        mock_repository: AsyncMock,
    ):
        """Test resuming a paused session."""
        session = create_mock_session(
            session_id=1,
            status=SessionStatus.PAUSED,
        )
        mock_repository.get_session_by_id.return_value = session
        mock_repository.update_session.side_effect = lambda s: s

        result = await session_manager.resume_session(1)

        # The session status should be changed to ACTIVE
        assert session.status == SessionStatus.ACTIVE
        mock_repository.update_session.assert_called_once()

    @pytest.mark.asyncio
    async def test_resume_session_completed_raises(
        self,
        session_manager: SessionManager,
        mock_repository: AsyncMock,
    ):
        """Test that resuming completed session raises error."""
        session = create_mock_session(
            session_id=1,
            status=SessionStatus.COMPLETED,
        )
        mock_repository.get_session_by_id.return_value = session

        with pytest.raises(InvalidSessionStateError) as exc_info:
            await session_manager.resume_session(1)

        assert exc_info.value.code == "REVIEW_003"
        assert "completed" in exc_info.value.details["current_status"]

    @pytest.mark.asyncio
    async def test_resume_session_not_found_raises(
        self,
        session_manager: SessionManager,
        mock_repository: AsyncMock,
    ):
        """Test that resuming non-existent session raises error."""
        mock_repository.get_session_by_id.return_value = None

        with pytest.raises(SessionNotFoundError) as exc_info:
            await session_manager.resume_session(999)

        assert exc_info.value.code == "REVIEW_001"
        assert exc_info.value.details["session_id"] == 999


class TestPauseSession:
    """Tests for SessionManager.pause_session()."""

    @pytest.mark.asyncio
    async def test_pause_session_success(
        self,
        session_manager: SessionManager,
        mock_repository: AsyncMock,
    ):
        """Test pausing an active session."""
        session = create_mock_session(
            session_id=1,
            status=SessionStatus.ACTIVE,
        )
        mock_repository.get_session_by_id.return_value = session
        mock_repository.update_session.return_value = session

        result = await session_manager.pause_session(1)

        assert session.status == SessionStatus.PAUSED
        mock_repository.update_session.assert_called_once()

    @pytest.mark.asyncio
    async def test_pause_session_not_active_raises(
        self,
        session_manager: SessionManager,
        mock_repository: AsyncMock,
    ):
        """Test that pausing non-active session raises error."""
        session = create_mock_session(
            session_id=1,
            status=SessionStatus.PAUSED,
        )
        mock_repository.get_session_by_id.return_value = session

        with pytest.raises(InvalidSessionStateError) as exc_info:
            await session_manager.pause_session(1)

        assert exc_info.value.code == "REVIEW_003"
        assert exc_info.value.details["operation"] == "pause"


class TestCompleteSession:
    """Tests for SessionManager.complete_session()."""

    @pytest.mark.asyncio
    async def test_complete_session_success(
        self,
        session_manager: SessionManager,
        mock_repository: AsyncMock,
    ):
        """Test completing a session with all items reviewed."""
        session = create_mock_session(
            session_id=1,
            status=SessionStatus.ACTIVE,
            total_items=10,
            items_reviewed=10,
        )
        mock_repository.get_session_by_id.return_value = session
        mock_repository.update_session.return_value = session

        result = await session_manager.complete_session(1)

        assert session.status == SessionStatus.COMPLETED
        assert session.completed_at is not None
        mock_repository.update_session.assert_called_once()

    @pytest.mark.asyncio
    async def test_complete_session_with_pending_items_raises(
        self,
        session_manager: SessionManager,
        mock_repository: AsyncMock,
    ):
        """Test that completing with pending items raises error."""
        session = create_mock_session(
            session_id=1,
            status=SessionStatus.ACTIVE,
            total_items=10,
            items_reviewed=5,
        )
        mock_repository.get_session_by_id.return_value = session

        with pytest.raises(PendingItemsError) as exc_info:
            await session_manager.complete_session(1)

        assert exc_info.value.code == "REVIEW_006"
        assert exc_info.value.details["pending_items"] == 5

    @pytest.mark.asyncio
    async def test_complete_session_force_with_pending_succeeds(
        self,
        session_manager: SessionManager,
        mock_repository: AsyncMock,
    ):
        """Test that force=True completes even with pending items."""
        session = create_mock_session(
            session_id=1,
            status=SessionStatus.ACTIVE,
            total_items=10,
            items_reviewed=5,
        )
        mock_repository.get_session_by_id.return_value = session
        mock_repository.update_session.return_value = session

        result = await session_manager.complete_session(1, force=True)

        assert session.status == SessionStatus.COMPLETED
        mock_repository.update_session.assert_called_once()

    @pytest.mark.asyncio
    async def test_complete_session_already_completed_raises(
        self,
        session_manager: SessionManager,
        mock_repository: AsyncMock,
    ):
        """Test that completing already completed session raises error."""
        session = create_mock_session(
            session_id=1,
            status=SessionStatus.COMPLETED,
        )
        mock_repository.get_session_by_id.return_value = session

        with pytest.raises(InvalidSessionStateError) as exc_info:
            await session_manager.complete_session(1)

        assert exc_info.value.code == "REVIEW_003"


class TestAbandonSession:
    """Tests for SessionManager.abandon_session()."""

    @pytest.mark.asyncio
    async def test_abandon_session_success(
        self,
        session_manager: SessionManager,
        mock_repository: AsyncMock,
    ):
        """Test abandoning an active session."""
        session = create_mock_session(
            session_id=1,
            status=SessionStatus.ACTIVE,
        )
        mock_repository.get_session_by_id.return_value = session
        mock_repository.update_session.return_value = session

        result = await session_manager.abandon_session(1)

        assert session.status == SessionStatus.ABANDONED
        mock_repository.update_session.assert_called_once()

    @pytest.mark.asyncio
    async def test_abandon_session_paused_success(
        self,
        session_manager: SessionManager,
        mock_repository: AsyncMock,
    ):
        """Test abandoning a paused session."""
        session = create_mock_session(
            session_id=1,
            status=SessionStatus.PAUSED,
        )
        mock_repository.get_session_by_id.return_value = session
        mock_repository.update_session.return_value = session

        result = await session_manager.abandon_session(1)

        assert session.status == SessionStatus.ABANDONED

    @pytest.mark.asyncio
    async def test_abandon_session_already_abandoned_raises(
        self,
        session_manager: SessionManager,
        mock_repository: AsyncMock,
    ):
        """Test that abandoning already abandoned session raises error."""
        session = create_mock_session(
            session_id=1,
            status=SessionStatus.ABANDONED,
        )
        mock_repository.get_session_by_id.return_value = session

        with pytest.raises(InvalidSessionStateError) as exc_info:
            await session_manager.abandon_session(1)

        assert exc_info.value.code == "REVIEW_003"


class TestUpdateProgress:
    """Tests for SessionManager.update_progress()."""

    @pytest.mark.asyncio
    async def test_update_progress_increments_counters(
        self,
        session_manager: SessionManager,
        mock_repository: AsyncMock,
    ):
        """Test that update_progress increments correct counters."""
        session = create_mock_session(
            session_id=1,
            status=SessionStatus.ACTIVE,
            items_reviewed=5,
            items_accepted=3,
            current_position=5,
        )
        mock_repository.get_session_by_id.return_value = session
        mock_repository.update_session.return_value = session

        result = await session_manager.update_progress(
            session_id=1,
            position=6,
            decision_type="accept",
        )

        assert session.items_reviewed == 6
        assert session.items_accepted == 4
        assert session.current_position == 6

    @pytest.mark.asyncio
    async def test_update_progress_reject(
        self,
        session_manager: SessionManager,
        mock_repository: AsyncMock,
    ):
        """Test update_progress with reject decision."""
        session = create_mock_session(
            session_id=1,
            status=SessionStatus.ACTIVE,
            items_reviewed=0,
            items_rejected=0,
        )
        mock_repository.get_session_by_id.return_value = session
        mock_repository.update_session.return_value = session

        await session_manager.update_progress(
            session_id=1,
            position=1,
            decision_type="reject",
        )

        assert session.items_rejected == 1

    @pytest.mark.asyncio
    async def test_update_progress_modify(
        self,
        session_manager: SessionManager,
        mock_repository: AsyncMock,
    ):
        """Test update_progress with modify decision."""
        session = create_mock_session(
            session_id=1,
            status=SessionStatus.ACTIVE,
            items_modified=2,
        )
        mock_repository.get_session_by_id.return_value = session
        mock_repository.update_session.return_value = session

        await session_manager.update_progress(
            session_id=1,
            position=1,
            decision_type="modify",
        )

        assert session.items_modified == 3

    @pytest.mark.asyncio
    async def test_update_progress_skip(
        self,
        session_manager: SessionManager,
        mock_repository: AsyncMock,
    ):
        """Test update_progress with skip decision."""
        session = create_mock_session(
            session_id=1,
            status=SessionStatus.ACTIVE,
            items_skipped=1,
        )
        mock_repository.get_session_by_id.return_value = session
        mock_repository.update_session.return_value = session

        await session_manager.update_progress(
            session_id=1,
            position=1,
            decision_type="skip",
        )

        assert session.items_skipped == 2

    @pytest.mark.asyncio
    async def test_update_progress_not_active_raises(
        self,
        session_manager: SessionManager,
        mock_repository: AsyncMock,
    ):
        """Test that update_progress on non-active session raises error."""
        session = create_mock_session(
            session_id=1,
            status=SessionStatus.PAUSED,
        )
        mock_repository.get_session_by_id.return_value = session

        with pytest.raises(InvalidSessionStateError) as exc_info:
            await session_manager.update_progress(
                session_id=1,
                position=1,
                decision_type="accept",
            )

        assert exc_info.value.details["operation"] == "update_progress"


class TestSessionStateTransitions:
    """Tests for valid session state transitions."""

    @pytest.mark.asyncio
    async def test_session_states_transitions(
        self,
        session_manager: SessionManager,
        mock_repository: AsyncMock,
    ):
        """Test full session lifecycle: active -> paused -> active -> completed."""
        session = create_mock_session(
            session_id=1,
            status=SessionStatus.ACTIVE,
            total_items=5,
            items_reviewed=0,
        )
        mock_repository.get_session_by_id.return_value = session
        mock_repository.update_session.side_effect = lambda s: s

        # Active -> Paused
        await session_manager.pause_session(1)
        assert session.status == SessionStatus.PAUSED

        # Paused -> Active
        await session_manager.resume_session(1)
        assert session.status == SessionStatus.ACTIVE

        # Simulate progress
        session.items_reviewed = 5
        session.total_items = 5

        # Active -> Completed
        await session_manager.complete_session(1)
        assert session.status == SessionStatus.COMPLETED
        assert session.completed_at is not None

    @pytest.mark.asyncio
    async def test_active_to_abandoned_transition(
        self,
        session_manager: SessionManager,
        mock_repository: AsyncMock,
    ):
        """Test active -> abandoned transition."""
        session = create_mock_session(
            session_id=1,
            status=SessionStatus.ACTIVE,
        )
        mock_repository.get_session_by_id.return_value = session
        mock_repository.update_session.side_effect = lambda s: s

        await session_manager.abandon_session(1)
        assert session.status == SessionStatus.ABANDONED

    @pytest.mark.asyncio
    async def test_paused_to_completed_transition(
        self,
        session_manager: SessionManager,
        mock_repository: AsyncMock,
    ):
        """Test paused -> completed transition."""
        session = create_mock_session(
            session_id=1,
            status=SessionStatus.PAUSED,
            total_items=10,
            items_reviewed=10,
        )
        mock_repository.get_session_by_id.return_value = session
        mock_repository.update_session.side_effect = lambda s: s

        await session_manager.complete_session(1)
        assert session.status == SessionStatus.COMPLETED


class TestSessionNotFound:
    """Tests for session not found scenarios."""

    @pytest.mark.asyncio
    async def test_pause_not_found(
        self,
        session_manager: SessionManager,
        mock_repository: AsyncMock,
    ):
        """Test pause raises SessionNotFoundError."""
        mock_repository.get_session_by_id.return_value = None

        with pytest.raises(SessionNotFoundError):
            await session_manager.pause_session(999)

    @pytest.mark.asyncio
    async def test_complete_not_found(
        self,
        session_manager: SessionManager,
        mock_repository: AsyncMock,
    ):
        """Test complete raises SessionNotFoundError."""
        mock_repository.get_session_by_id.return_value = None

        with pytest.raises(SessionNotFoundError):
            await session_manager.complete_session(999)

    @pytest.mark.asyncio
    async def test_abandon_not_found(
        self,
        session_manager: SessionManager,
        mock_repository: AsyncMock,
    ):
        """Test abandon raises SessionNotFoundError."""
        mock_repository.get_session_by_id.return_value = None

        with pytest.raises(SessionNotFoundError):
            await session_manager.abandon_session(999)

    @pytest.mark.asyncio
    async def test_update_progress_not_found(
        self,
        session_manager: SessionManager,
        mock_repository: AsyncMock,
    ):
        """Test update_progress raises SessionNotFoundError."""
        mock_repository.get_session_by_id.return_value = None

        with pytest.raises(SessionNotFoundError):
            await session_manager.update_progress(999, 1, "accept")
