"""Unit tests for the search session module.

Tests for SessionManager and SearchSessionInfo for tracking
user search activity, history, and session management.
"""

from datetime import datetime, timezone, timedelta
from unittest.mock import AsyncMock, MagicMock, patch
import uuid

import pytest

from penf_lib.search.session import (
    SessionManager,
    SearchSessionInfo,
)


# =============================================================================
# FIXTURES
# =============================================================================


@pytest.fixture
def mock_db_session():
    """Create a mock database session."""
    return AsyncMock()


@pytest.fixture
def mock_search_repository():
    """Create a mock search repository."""
    repo = MagicMock()
    repo.get_active_session = AsyncMock()
    repo.create_session = AsyncMock()
    repo.update_session_activity = AsyncMock()
    repo.close_session = AsyncMock()
    repo.get_recent_queries = AsyncMock()
    repo.record_query = AsyncMock()
    repo.cleanup_expired_sessions = AsyncMock()
    return repo


@pytest.fixture
def session_manager(mock_db_session):
    """Create a SessionManager instance for testing."""
    return SessionManager(mock_db_session, "test-tenant-id")


@pytest.fixture
def sample_session_data():
    """Create sample session data for testing."""
    now = datetime.now(timezone.utc)
    return {
        "id": uuid.uuid4(),
        "session_id": "search-abc123def456",
        "user_email": "user@example.com",
        "is_active": True,
        "started_at": now - timedelta(hours=1),
        "last_activity_at": now - timedelta(minutes=5),
        "expires_at": now + timedelta(hours=23),
        "session_context": "{}",
        "total_queries": 5,
        "successful_searches": 4,
    }


# =============================================================================
# SEARCH SESSION INFO TESTS
# =============================================================================


class TestSearchSessionInfo:
    """Tests for SearchSessionInfo dataclass."""

    def test_session_info_creation(self):
        """Test creating a SearchSessionInfo instance."""
        now = datetime.now(timezone.utc)
        expires = now + timedelta(hours=24)

        info = SearchSessionInfo(
            session_id="test-session-123",
            user_email="user@example.com",
            started_at=now,
            last_activity_at=now,
            expires_at=expires,
            total_queries=0,
            is_active=True,
        )

        assert info.session_id == "test-session-123"
        assert info.user_email == "user@example.com"
        assert info.started_at == now
        assert info.total_queries == 0
        assert info.is_active is True

    def test_session_info_with_queries(self):
        """Test SearchSessionInfo with query count."""
        now = datetime.now(timezone.utc)

        info = SearchSessionInfo(
            session_id="active-session",
            user_email="active@example.com",
            started_at=now - timedelta(hours=2),
            last_activity_at=now - timedelta(minutes=10),
            expires_at=now + timedelta(hours=22),
            total_queries=15,
            is_active=True,
        )

        assert info.total_queries == 15
        assert info.is_active is True


# =============================================================================
# SESSION MANAGER TESTS
# =============================================================================


class TestSessionManager:
    """Tests for SessionManager class."""

    def test_session_manager_initialization(self, mock_db_session):
        """Test SessionManager initialization."""
        manager = SessionManager(mock_db_session, "tenant-123")

        assert manager.db_session == mock_db_session
        assert manager.tenant_id == "tenant-123"
        assert manager.SESSION_DURATION_HOURS == 24

    def test_tenant_id_to_uuid_with_valid_uuid(self, session_manager):
        """Test _to_uuid with valid UUID string."""
        manager = SessionManager(
            session_manager.db_session,
            "12345678-1234-5678-1234-567812345678",
        )
        result = manager._to_uuid()
        assert isinstance(result, uuid.UUID)
        assert str(result) == "12345678-1234-5678-1234-567812345678"

    def test_tenant_id_to_uuid_with_name(self, session_manager):
        """Test _to_uuid with non-UUID tenant name."""
        result = session_manager._to_uuid()
        assert isinstance(result, uuid.UUID)
        # Should be deterministic (same input = same UUID)
        result2 = session_manager._to_uuid()
        assert result == result2

    @pytest.mark.asyncio
    async def test_get_or_create_session_creates_new(
        self, session_manager, mock_search_repository, sample_session_data
    ):
        """Test getting or creating a session when none exists."""
        mock_search_repository.get_active_session.return_value = None
        mock_search_repository.create_session.return_value = sample_session_data

        # Set the private _repository directly to bypass the property
        session_manager._repository = mock_search_repository
        result = await session_manager.get_or_create_session("user@example.com")

        assert isinstance(result, SearchSessionInfo)
        assert result.user_email == "user@example.com"
        mock_search_repository.create_session.assert_called_once()

    @pytest.mark.asyncio
    async def test_get_or_create_session_returns_existing(
        self, session_manager, mock_search_repository, sample_session_data
    ):
        """Test getting existing session when one exists."""
        mock_search_repository.get_active_session.return_value = sample_session_data

        session_manager._repository = mock_search_repository
        result = await session_manager.get_or_create_session("user@example.com")

        assert isinstance(result, SearchSessionInfo)
        assert result.total_queries == 5
        mock_search_repository.update_session_activity.assert_called_once()
        mock_search_repository.create_session.assert_not_called()

    @pytest.mark.asyncio
    async def test_end_session(self, session_manager, mock_search_repository):
        """Test ending a search session."""
        mock_search_repository.close_session.return_value = None

        session_manager._repository = mock_search_repository
        result = await session_manager.end_session(
            "12345678-1234-5678-1234-567812345678"
        )

        assert result is True
        mock_search_repository.close_session.assert_called_once()

    @pytest.mark.asyncio
    async def test_end_session_with_search_prefix(
        self, session_manager, mock_search_repository
    ):
        """Test ending session with search- prefix format."""
        mock_search_repository.close_session.return_value = None

        session_manager._repository = mock_search_repository
        result = await session_manager.end_session("search-abc123def456")

        assert result is True
        mock_search_repository.close_session.assert_called_once()

    @pytest.mark.asyncio
    async def test_get_session_history(
        self, session_manager, mock_search_repository
    ):
        """Test getting session history."""
        now = datetime.now(timezone.utc)
        mock_search_repository.get_recent_queries.return_value = [
            {
                "query_text": "customer issues",
                "result_count": 10,
                "execution_time_ms": 150,
                "created_at": now - timedelta(minutes=5),
            },
            {
                "query_text": "deployment problems",
                "result_count": 5,
                "execution_time_ms": 120,
                "created_at": now - timedelta(minutes=10),
            },
        ]

        session_manager._repository = mock_search_repository
        result = await session_manager.get_session_history(
            "user@example.com", limit=10
        )

        assert len(result) == 2
        assert result[0]["query_text"] == "customer issues"
        assert result[0]["result_count"] == 10
        assert result[1]["query_text"] == "deployment problems"

    @pytest.mark.asyncio
    async def test_record_query(self, session_manager, mock_search_repository):
        """Test recording a query for history."""
        now = datetime.now(timezone.utc)
        mock_search_repository.record_query.return_value = {
            "id": uuid.uuid4(),
            "query_text": "test query",
            "execution_time_ms": 100,
            "result_count": 5,
            "cache_hit": False,
            "search_strategy": "hybrid",
            "created_at": now,
        }

        session_manager._repository = mock_search_repository
        result = await session_manager.record_query(
            session_id="search-abc123def456",
            query_text="test query",
            filters={"content_types": ["email"]},
            execution_time_ms=100,
            result_count=5,
            cache_hit=False,
            search_strategy="hybrid",
        )

        assert result["query_text"] == "test query"
        mock_search_repository.record_query.assert_called_once()

    @pytest.mark.asyncio
    async def test_cleanup_expired(self, session_manager, mock_search_repository):
        """Test cleaning up expired sessions."""
        mock_search_repository.cleanup_expired_sessions.return_value = 3

        session_manager._repository = mock_search_repository
        result = await session_manager.cleanup_expired()

        assert result == 3
        mock_search_repository.cleanup_expired_sessions.assert_called_once()


# =============================================================================
# SESSION LIFECYCLE TESTS
# =============================================================================


class TestSessionLifecycle:
    """Integration tests for session lifecycle."""

    @pytest.mark.asyncio
    async def test_session_duration_is_24_hours(self, mock_db_session):
        """Test that session duration is 24 hours."""
        manager = SessionManager(mock_db_session, "tenant")
        assert manager.SESSION_DURATION_HOURS == 24

    def test_session_info_tracks_activity(self):
        """Test that session info tracks last activity."""
        start = datetime.now(timezone.utc) - timedelta(hours=2)
        last_activity = datetime.now(timezone.utc) - timedelta(minutes=5)

        info = SearchSessionInfo(
            session_id="activity-test",
            user_email="user@example.com",
            started_at=start,
            last_activity_at=last_activity,
            expires_at=start + timedelta(hours=24),
            total_queries=10,
            is_active=True,
        )

        assert info.started_at < info.last_activity_at
        assert info.last_activity_at < info.expires_at
