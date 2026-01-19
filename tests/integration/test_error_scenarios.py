"""Integration tests for Gmail error handling and recovery scenarios."""

import pytest
import asyncio
from unittest.mock import Mock, AsyncMock, patch, MagicMock
from datetime import datetime, timedelta
from typing import Dict, Any
import json

from googleapiclient.errors import HttpError
from google.auth.exceptions import RefreshError
from sqlalchemy.exc import OperationalError
import asyncpg
from aiohttp import ClientError

from penf_lib.connectors.gmail.error_handling import (
    GmailErrorHandler, GmailErrorClassifier, ErrorCategory, ErrorSeverity,
    ErrorContext, RetryConfig, CircuitBreaker, gmail_error_handler
)
from penf_lib.connectors.gmail.recovery import (
    OAuthTokenRecovery, SyncRecovery, WebhookRecovery, SystemHealthRecovery,
    RecoveryStatus, system_recovery
)
from penf_lib.connectors.gmail.client import GmailClient
from penf_lib.connectors.gmail.auth import GmailAuthenticator


@pytest.fixture
def error_handler():
    """Create fresh error handler for testing."""
    return GmailErrorHandler()


@pytest.fixture
def mock_gmail_client():
    """Create mock Gmail client for testing."""
    mock_client = Mock(spec=GmailClient)
    mock_client.connection_id = 1
    return mock_client


@pytest.fixture
def sample_http_error():
    """Create a sample HTTP error for testing."""
    resp = Mock()
    resp.status = 429
    resp.reason = "Too Many Requests"
    return HttpError(resp, b'{"error": {"code": 429, "message": "Rate limit exceeded"}}')


class TestGmailErrorClassifier:
    """Test error classification and context creation."""

    def test_classify_oauth_refresh_error(self):
        """Test classification of OAuth refresh errors."""
        error = RefreshError("Token refresh failed")
        context = GmailErrorClassifier.classify_error(error, "token_refresh")

        assert context.category == ErrorCategory.OAUTH_TOKEN
        assert context.severity == ErrorSeverity.HIGH
        assert "refresh_token" in context.recovery_actions
        assert "re_authenticate" in context.recovery_actions

    def test_classify_http_401_error(self, sample_http_error):
        """Test classification of HTTP 401 authentication errors."""
        sample_http_error.resp.status = 401
        context = GmailErrorClassifier.classify_error(sample_http_error, "api_call")

        assert context.category == ErrorCategory.AUTHENTICATION
        assert context.severity == ErrorSeverity.HIGH
        assert "refresh_token" in context.recovery_actions

    def test_classify_http_429_rate_limit_error(self, sample_http_error):
        """Test classification of HTTP 429 rate limit errors."""
        context = GmailErrorClassifier.classify_error(sample_http_error, "api_call")

        assert context.category == ErrorCategory.API_RATE_LIMIT
        assert context.severity == ErrorSeverity.MEDIUM
        assert context.max_attempts == 10
        assert "exponential_backoff" in context.recovery_actions

    def test_classify_http_403_quota_error(self):
        """Test classification of HTTP 403 quota exceeded errors."""
        resp = Mock()
        resp.status = 403
        error = HttpError(resp, b'{"error": {"errors": [{"reason": "quotaExceeded"}]}}')
        error.error_details = [{"reason": "quotaExceeded"}]

        context = GmailErrorClassifier.classify_error(error, "api_call")

        assert context.category == ErrorCategory.API_QUOTA
        assert context.severity == ErrorSeverity.MEDIUM
        assert context.backoff_multiplier == 5.0

    def test_classify_network_error(self):
        """Test classification of network connectivity errors."""
        error = ClientError("Connection failed")
        context = GmailErrorClassifier.classify_error(error, "api_call")

        assert context.category == ErrorCategory.NETWORK
        assert context.severity == ErrorSeverity.MEDIUM
        assert "retry_with_backoff" in context.recovery_actions

    def test_classify_database_error(self):
        """Test classification of database errors."""
        error = OperationalError("Database connection failed", None, None)
        context = GmailErrorClassifier.classify_error(error, "database_operation")

        assert context.category == ErrorCategory.DATABASE
        assert context.severity == ErrorSeverity.CRITICAL
        assert "database_reconnect" in context.recovery_actions


class TestCircuitBreaker:
    """Test circuit breaker functionality."""

    @pytest.mark.asyncio
    async def test_circuit_breaker_success(self):
        """Test circuit breaker with successful calls."""
        cb = CircuitBreaker(failure_threshold=3)

        async def success_func():
            return "success"

        result = await cb.call(success_func)
        assert result == "success"
        assert cb.state == "CLOSED"
        assert cb.failure_count == 0

    @pytest.mark.asyncio
    async def test_circuit_breaker_failure_threshold(self):
        """Test circuit breaker opening after failure threshold."""
        cb = CircuitBreaker(failure_threshold=2, recovery_timeout=0.1)

        async def failing_func():
            raise Exception("Test error")

        # First failure
        with pytest.raises(Exception):
            await cb.call(failing_func)
        assert cb.state == "CLOSED"
        assert cb.failure_count == 1

        # Second failure - should open circuit
        with pytest.raises(Exception):
            await cb.call(failing_func)
        assert cb.state == "OPEN"
        assert cb.failure_count == 2

        # Third call should fail immediately due to open circuit
        with pytest.raises(Exception, match="Circuit breaker OPEN"):
            await cb.call(failing_func)

    @pytest.mark.asyncio
    async def test_circuit_breaker_recovery(self):
        """Test circuit breaker recovery after timeout."""
        cb = CircuitBreaker(failure_threshold=2, recovery_timeout=0.01)

        async def failing_func():
            raise Exception("Test error")

        # Trigger circuit breaker
        for _ in range(2):
            with pytest.raises(Exception):
                await cb.call(failing_func)

        assert cb.state == "OPEN"

        # Wait for recovery timeout
        await asyncio.sleep(0.02)

        # Next call should attempt recovery (half-open state)
        async def success_func():
            return "recovered"

        result = await cb.call(success_func)
        assert result == "recovered"
        assert cb.state == "CLOSED"
        assert cb.failure_count == 0


class TestGmailErrorHandler:
    """Test comprehensive error handling functionality."""

    @pytest.mark.asyncio
    async def test_handle_with_retry_success(self, error_handler):
        """Test successful operation without retries."""
        call_count = 0

        async def success_func():
            nonlocal call_count
            call_count += 1
            return "success"

        result = await error_handler.handle_with_retry(
            success_func, "test_operation", connection_id=1
        )

        assert result == "success"
        assert call_count == 1

    @pytest.mark.asyncio
    async def test_handle_with_retry_eventual_success(self, error_handler):
        """Test operation succeeding after retries."""
        call_count = 0

        async def eventually_success_func():
            nonlocal call_count
            call_count += 1
            if call_count < 3:
                raise Exception("Temporary error")
            return "success"

        config = RetryConfig(max_attempts=5, base_delay=0.01)
        result = await error_handler.handle_with_retry(
            eventually_success_func, "test_operation", connection_id=1, retry_config=config
        )

        assert result == "success"
        assert call_count == 3

    @pytest.mark.asyncio
    async def test_handle_with_retry_permanent_failure(self, error_handler):
        """Test operation failing permanently after max retries."""
        call_count = 0

        async def failing_func():
            nonlocal call_count
            call_count += 1
            raise Exception("Permanent error")

        config = RetryConfig(max_attempts=3, base_delay=0.01)

        with pytest.raises(Exception, match="Permanent error"):
            await error_handler.handle_with_retry(
                failing_func, "test_operation", connection_id=1, retry_config=config
            )

        assert call_count == 3

    @pytest.mark.asyncio
    async def test_handle_with_retry_rate_limit_backoff(self, error_handler):
        """Test appropriate backoff for rate limit errors."""
        call_count = 0

        async def rate_limit_func():
            nonlocal call_count
            call_count += 1
            if call_count < 3:
                resp = Mock()
                resp.status = 429
                raise HttpError(resp, b'{"error": {"code": 429}}')
            return "success"

        config = RetryConfig(max_attempts=5, base_delay=0.01)
        start_time = datetime.utcnow()

        result = await error_handler.handle_with_retry(
            rate_limit_func, "test_operation", connection_id=1, retry_config=config
        )

        duration = (datetime.utcnow() - start_time).total_seconds()
        assert result == "success"
        assert call_count == 3
        # Should have some delay due to backoff
        assert duration > 0.01

    @pytest.mark.asyncio
    async def test_should_not_retry_permanent_errors(self, error_handler):
        """Test that certain errors are not retried."""
        call_count = 0

        async def bad_request_func():
            nonlocal call_count
            call_count += 1
            resp = Mock()
            resp.status = 400
            raise HttpError(resp, b'{"error": {"code": 400, "message": "Bad Request"}}')

        config = RetryConfig(max_attempts=5, base_delay=0.01)

        with pytest.raises(HttpError):
            await error_handler.handle_with_retry(
                bad_request_func, "test_operation", connection_id=1, retry_config=config
            )

        # Should not retry 400 errors
        assert call_count == 1


class TestOAuthTokenRecovery:
    """Test OAuth token recovery functionality."""

    @pytest.fixture
    def token_recovery(self):
        """Create OAuth token recovery instance."""
        mock_authenticator = Mock(spec=GmailAuthenticator)
        return OAuthTokenRecovery(mock_authenticator)

    @pytest.mark.asyncio
    @patch('penf_lib.connectors.gmail.recovery.db_manager.get_session')
    async def test_recover_valid_tokens_skipped(self, mock_session, token_recovery):
        """Test recovery skips connections with already valid tokens."""
        # Mock database session and connection
        mock_conn = Mock()
        mock_conn.id = 1
        mock_conn.encrypted_credentials = b"encrypted_data"

        mock_session_instance = AsyncMock()
        mock_session_instance.execute.return_value.scalar_one_or_none.return_value = mock_conn
        mock_session.return_value.__aenter__.return_value = mock_session_instance

        # Mock valid credentials
        token_recovery.authenticator.decrypt_credentials.return_value = Mock()
        token_recovery.authenticator.validate_credentials.return_value = True

        result = await token_recovery.recover_connection_tokens(1)

        assert result.status == RecoveryStatus.SKIPPED
        assert "already valid" in result.message
        assert "validated_existing_credentials" in result.actions_taken

    @pytest.mark.asyncio
    @patch('penf_lib.connectors.gmail.recovery.db_manager.get_session')
    async def test_recover_refresh_token_success(self, mock_session, token_recovery):
        """Test successful token refresh."""
        # Mock database session and connection
        mock_conn = Mock()
        mock_conn.id = 1
        mock_conn.encrypted_credentials = b"encrypted_data"
        mock_conn.token_refresh_count = 0

        mock_session_instance = AsyncMock()
        mock_session_instance.execute.return_value.scalar_one_or_none.return_value = mock_conn
        mock_session.return_value.__aenter__.return_value = mock_session_instance

        # Mock invalid credentials that can be refreshed
        mock_credentials = Mock()
        mock_credentials.refresh_token = "refresh_token"

        token_recovery.authenticator.decrypt_credentials.return_value = mock_credentials
        token_recovery.authenticator.validate_credentials.side_effect = [False, True]  # Invalid, then valid
        token_recovery.authenticator.refresh_credentials.return_value = mock_credentials
        token_recovery.authenticator.encrypt_credentials.return_value = b"new_encrypted_data"

        result = await token_recovery.recover_connection_tokens(1)

        assert result.status == RecoveryStatus.COMPLETED
        assert "Successfully refreshed" in result.message
        assert "refreshed_access_token" in result.actions_taken
        assert "updated_stored_credentials" in result.actions_taken

    @pytest.mark.asyncio
    @patch('penf_lib.connectors.gmail.recovery.db_manager.get_session')
    async def test_recover_no_refresh_token_failure(self, mock_session, token_recovery):
        """Test recovery failure when no refresh token is available."""
        # Mock database session and connection
        mock_conn = Mock()
        mock_conn.id = 1
        mock_conn.encrypted_credentials = b"encrypted_data"

        mock_session_instance = AsyncMock()
        mock_session_instance.execute.return_value.scalar_one_or_none.return_value = mock_conn
        mock_session.return_value.__aenter__.return_value = mock_session_instance

        # Mock invalid credentials without refresh token
        mock_credentials = Mock()
        mock_credentials.refresh_token = None

        token_recovery.authenticator.decrypt_credentials.return_value = mock_credentials
        token_recovery.authenticator.validate_credentials.return_value = False

        result = await token_recovery.recover_connection_tokens(1)

        assert result.status == RecoveryStatus.FAILED
        assert "re-authentication required" in result.message
        assert "marked_for_reauth" in result.actions_taken
        assert result.metadata["requires_user_action"] is True


class TestSyncRecovery:
    """Test sync operation recovery functionality."""

    @pytest.fixture
    def sync_recovery(self):
        """Create sync recovery instance."""
        return SyncRecovery()

    @pytest.mark.asyncio
    @patch('penf_lib.connectors.gmail.recovery.db_manager.get_session')
    async def test_recover_no_stuck_messages(self, mock_session, sync_recovery):
        """Test recovery when no stuck messages are found."""
        mock_session_instance = AsyncMock()
        mock_session_instance.execute.return_value.scalars.return_value.all.return_value = []
        mock_session.return_value.__aenter__.return_value = mock_session_instance

        result = await sync_recovery.recover_interrupted_sync(1)

        assert result.status == RecoveryStatus.SKIPPED
        assert "No stuck messages" in result.message

    @pytest.mark.asyncio
    @patch('penf_lib.connectors.gmail.recovery.db_manager.get_session')
    async def test_recover_stuck_messages_success(self, mock_session, sync_recovery):
        """Test successful recovery of stuck messages."""
        # Mock stuck messages and threads
        mock_message1 = Mock()
        mock_message1.gmail_message_id = "msg1"
        mock_message1.processing_status = "processing"

        mock_message2 = Mock()
        mock_message2.gmail_message_id = "msg2"
        mock_message2.processing_status = "processing"

        mock_thread1 = Mock()
        mock_thread1.gmail_thread_id = "thread1"
        mock_thread1.processing_status = "processing"

        mock_session_instance = AsyncMock()
        # First call returns stuck messages, second call returns stuck threads
        mock_session_instance.execute.return_value.scalars.return_value.all.side_effect = [
            [mock_message1, mock_message2],  # stuck messages
            [mock_thread1]                   # stuck threads
        ]
        mock_session.return_value.__aenter__.return_value = mock_session_instance

        result = await sync_recovery.recover_interrupted_sync(1)

        assert result.status == RecoveryStatus.COMPLETED
        assert result.metadata["messages_reset"] == 2
        assert result.metadata["threads_reset"] == 1
        assert "reset_message_msg1" in result.actions_taken
        assert "reset_message_msg2" in result.actions_taken
        assert "reset_thread_thread1" in result.actions_taken

        # Verify messages were reset
        assert mock_message1.processing_status == "pending"
        assert mock_message2.processing_status == "pending"
        assert mock_thread1.processing_status == "pending"


class TestWebhookRecovery:
    """Test webhook subscription recovery functionality."""

    @pytest.fixture
    def webhook_recovery(self):
        """Create webhook recovery instance."""
        return WebhookRecovery()

    @pytest.mark.asyncio
    @patch('penf_lib.connectors.gmail.recovery.db_manager.get_session')
    @patch('penf_lib.connectors.gmail.recovery.create_sync_manager')
    async def test_recover_expired_webhooks(self, mock_create_sync, mock_session, webhook_recovery):
        """Test recovery of expired webhook subscriptions."""
        # Mock expired connection
        mock_conn = Mock()
        mock_conn.id = 1
        mock_conn.project_id = "test-project"
        mock_conn.push_subscription_expiry = datetime.utcnow() - timedelta(hours=1)  # Expired

        mock_session_instance = AsyncMock()
        mock_session_instance.execute.return_value.scalars.return_value.all.return_value = [mock_conn]
        mock_session.return_value.__aenter__.return_value = mock_session_instance

        # Mock sync manager
        mock_sync_manager = AsyncMock()
        mock_sync_manager.setup_push_notifications.return_value = {"historyId": "12345", "expiration": "1609459200000"}
        mock_create_sync.return_value = mock_sync_manager

        result = await webhook_recovery.recover_webhook_subscriptions()

        assert result.status == RecoveryStatus.COMPLETED
        assert result.metadata["subscriptions_recovered"] == 1
        assert "renewed_webhook_1" in result.actions_taken

        # Verify setup_push_notifications was called
        mock_sync_manager.setup_push_notifications.assert_called_once()


class TestSystemHealthRecovery:
    """Test comprehensive system health monitoring and recovery."""

    @pytest.fixture
    def health_recovery(self):
        """Create system health recovery instance."""
        return SystemHealthRecovery()

    @pytest.mark.asyncio
    @patch('penf_lib.connectors.gmail.recovery.db_manager.get_session')
    async def test_health_check_healthy_system(self, mock_session, health_recovery):
        """Test health check on a healthy system."""
        # Mock healthy database connection
        mock_session_instance = AsyncMock()
        mock_session_instance.execute.return_value = None  # Success

        # Mock healthy connections
        mock_session_instance.execute.return_value.scalars.return_value.all.side_effect = [
            [],  # No active connections with problems
            [],  # No stuck messages
        ]

        mock_session.return_value.__aenter__.return_value = mock_session_instance

        # Mock count methods
        health_recovery._count_total_messages = AsyncMock(return_value=1000)
        health_recovery._count_messages_today = AsyncMock(return_value=50)

        result = await health_recovery.perform_health_check()

        assert result["overall_status"] == "healthy"
        assert result["components"]["database"]["status"] == "healthy"
        assert result["components"]["connections"]["status"] == "healthy"
        assert result["statistics"]["total_messages"] == 1000
        assert result["statistics"]["messages_today"] == 50

    @pytest.mark.asyncio
    @patch('penf_lib.connectors.gmail.recovery.db_manager.get_session')
    async def test_health_check_degraded_system(self, mock_session, health_recovery):
        """Test health check on a system with some issues."""
        # Mock database connection
        mock_session_instance = AsyncMock()
        mock_session_instance.execute.return_value = None  # Database OK

        # Mock connections with problems
        mock_problem_conn = Mock()
        mock_problem_conn.id = 1
        mock_stuck_message = Mock()
        mock_stuck_message.id = 1

        mock_session_instance.execute.return_value.scalars.return_value.all.side_effect = [
            [Mock(), Mock(), Mock()],  # 3 active connections
            [mock_problem_conn],       # 1 problem connection
            [mock_stuck_message],      # 1 stuck message
        ]

        mock_session.return_value.__aenter__.return_value = mock_session_instance

        # Mock count methods
        health_recovery._count_total_messages = AsyncMock(return_value=1000)
        health_recovery._count_messages_today = AsyncMock(return_value=50)

        result = await health_recovery.perform_health_check()

        assert result["overall_status"] == "degraded"
        assert result["components"]["connections"]["status"] == "degraded"
        assert result["components"]["sync_operations"]["status"] == "degraded"
        assert len(result["recommendations"]) > 0
        assert any("stuck operations" in rec for rec in result["recommendations"])
        assert any("OAuth token recovery" in rec for rec in result["recommendations"])

    @pytest.mark.asyncio
    @patch.object(SystemHealthRecovery, 'oauth_recovery')
    @patch.object(SystemHealthRecovery, 'sync_recovery')
    @patch.object(SystemHealthRecovery, 'webhook_recovery')
    async def test_automated_recovery_success(self, mock_webhook, mock_sync, mock_oauth, health_recovery):
        """Test successful automated recovery."""
        # Mock successful recovery operations
        mock_oauth.recover_all_invalid_tokens.return_value = [
            Mock(status=RecoveryStatus.COMPLETED),
            Mock(status=RecoveryStatus.COMPLETED)
        ]

        mock_sync.recover_interrupted_sync.return_value = Mock(status=RecoveryStatus.COMPLETED)
        mock_webhook.recover_webhook_subscriptions.return_value = Mock(status=RecoveryStatus.COMPLETED)

        # Mock database connections
        with patch('penf_lib.connectors.gmail.recovery.db_manager.get_session') as mock_session:
            mock_session_instance = AsyncMock()
            mock_session_instance.execute.return_value.scalars.return_value.all.return_value = [Mock(id=1)]
            mock_session.return_value.__aenter__.return_value = mock_session_instance

            result = await health_recovery.run_automated_recovery()

        assert result["summary"]["total_operations"] == 3
        assert result["summary"]["successful_operations"] == 3
        assert result["summary"]["failed_operations"] == 0
        assert "oauth_recovery" in result["operations"]
        assert "sync_recovery" in result["operations"]
        assert "webhook_recovery" in result["operations"]

    @pytest.mark.asyncio
    async def test_error_statistics_tracking(self, error_handler):
        """Test error statistics tracking and threshold alerts."""
        # Simulate multiple errors
        for _ in range(3):
            error_context = ErrorContext(
                error=Exception("Test error"),
                category=ErrorCategory.API_RATE_LIMIT,
                severity=ErrorSeverity.MEDIUM,
                operation="test_operation"
            )
            error_handler._update_error_stats(error_context)

        stats = error_handler.get_error_statistics()
        assert "api_rate_limit" in stats["error_stats"]
        assert stats["error_stats"]["api_rate_limit"]["medium"] == 3

        # Test statistics reset
        error_handler.reset_error_statistics()
        stats = error_handler.get_error_statistics()
        assert len(stats["error_stats"]) == 0


class TestIntegrationScenarios:
    """Integration tests for complex error scenarios."""

    @pytest.mark.asyncio
    @patch('penf_lib.connectors.gmail.client.GmailClient._make_api_call')
    async def test_api_call_with_multiple_error_types(self, mock_api_call, mock_gmail_client):
        """Test handling multiple types of errors in sequence."""
        # Configure mock to fail with different errors, then succeed
        call_count = 0

        async def mock_call(*args, **kwargs):
            nonlocal call_count
            call_count += 1

            if call_count == 1:
                # First call: rate limit error
                resp = Mock()
                resp.status = 429
                raise HttpError(resp, b'{"error": {"code": 429}}')
            elif call_count == 2:
                # Second call: network error
                raise ClientError("Network timeout")
            elif call_count == 3:
                # Third call: success
                return {"messages": []}
            else:
                raise Exception("Unexpected call")

        mock_api_call.side_effect = mock_call

        # This should eventually succeed after handling different error types
        result = await gmail_error_handler.handle_with_retry(
            mock_gmail_client.list_messages,
            "test_multi_error",
            connection_id=1,
            retry_config=RetryConfig(max_attempts=5, base_delay=0.01)
        )

        # Should have succeeded on third attempt
        assert call_count == 3

    @pytest.mark.asyncio
    async def test_circuit_breaker_integration_with_error_handler(self):
        """Test circuit breaker integration with error handler."""
        error_handler = GmailErrorHandler()

        call_count = 0

        async def failing_operation():
            nonlocal call_count
            call_count += 1
            raise Exception("Persistent failure")

        # This should eventually trigger circuit breaker
        with pytest.raises(Exception):
            await error_handler.handle_with_retry(
                failing_operation,
                "circuit_breaker_test",
                retry_config=RetryConfig(max_attempts=3, base_delay=0.01)
            )

        # Check that circuit breaker was activated
        cb = error_handler.get_circuit_breaker("circuit_breaker_test")
        assert cb.state == "OPEN"
        assert cb.failure_count >= 3

    @pytest.mark.asyncio
    async def test_end_to_end_recovery_scenario(self):
        """Test end-to-end recovery scenario with multiple components."""
        # This test would simulate a complete system recovery scenario
        # including OAuth token refresh, sync recovery, and webhook restoration

        # Mock the entire recovery flow
        recovery = SystemHealthRecovery()

        with patch('penf_lib.connectors.gmail.recovery.db_manager.get_session') as mock_session:
            # Mock database interactions
            mock_session_instance = AsyncMock()
            mock_session.return_value.__aenter__.return_value = mock_session_instance

            # Mock successful health check
            recovery._count_total_messages = AsyncMock(return_value=1000)
            recovery._count_messages_today = AsyncMock(return_value=50)

            # Mock problematic connections
            mock_session_instance.execute.return_value.scalars.return_value.all.side_effect = [
                [Mock(id=1), Mock(id=2)],  # active connections
                [Mock(id=1)],              # problem connections
                []                         # no stuck messages
            ]

            health_check = await recovery.perform_health_check()

            # Should detect degraded system
            assert health_check["overall_status"] == "degraded"
            assert len(health_check["recommendations"]) > 0


@pytest.mark.asyncio
async def test_comprehensive_error_handling_integration():
    """Test the complete error handling system integration."""
    # Test that all error handling components work together

    # Reset error handler state
    gmail_error_handler.reset_error_statistics()

    # Test various error scenarios
    test_errors = [
        (RefreshError("Token expired"), ErrorCategory.OAUTH_TOKEN),
        (HttpError(Mock(status=429), b"{}"), ErrorCategory.API_RATE_LIMIT),
        (OperationalError("DB error", None, None), ErrorCategory.DATABASE),
        (ClientError("Network error"), ErrorCategory.NETWORK),
    ]

    for error, expected_category in test_errors:
        context = GmailErrorClassifier.classify_error(error, "test_operation")
        assert context.category == expected_category

        # Update stats
        gmail_error_handler._update_error_stats(context)

    # Check that error statistics were properly tracked
    stats = gmail_error_handler.get_error_statistics()
    assert len(stats["error_stats"]) == 4  # All error categories tracked

    # Test recovery system initialization
    recovery = SystemHealthRecovery()
    assert recovery.oauth_recovery is not None
    assert recovery.sync_recovery is not None
    assert recovery.webhook_recovery is not None