"""Gmail Integration Error Handling and Recovery System.

This module provides comprehensive error handling, retry logic, and recovery
mechanisms for all Gmail integration operations including OAuth2, API calls,
webhooks, database operations, and multi-account coordination.
"""

import asyncio
import logging
import time
from datetime import datetime, timedelta
from enum import Enum
from typing import Any, Dict, List, Optional, Type, Union, Callable, Awaitable
from dataclasses import dataclass, field
import traceback
import json

from googleapiclient.errors import HttpError
from google.auth.exceptions import RefreshError
from sqlalchemy.exc import SQLAlchemyError, OperationalError, TimeoutError as SQLTimeoutError
from aiohttp import ClientError, ClientTimeout, ClientConnectorError
import asyncpg
from redis import RedisError

from ...storage.database import db_manager
from ...events.exceptions import EventPublishError


logger = logging.getLogger(__name__)


class ErrorSeverity(Enum):
    """Error severity levels."""
    LOW = "low"
    MEDIUM = "medium"
    HIGH = "high"
    CRITICAL = "critical"


class ErrorCategory(Enum):
    """Categories of errors for different handling strategies."""
    OAUTH_TOKEN = "oauth_token"
    API_RATE_LIMIT = "api_rate_limit"
    API_QUOTA = "api_quota"
    NETWORK = "network"
    DATABASE = "database"
    WEBHOOK = "webhook"
    ATTACHMENT = "attachment"
    COORDINATION = "coordination"
    AUTHENTICATION = "authentication"
    PERMISSION = "permission"
    DATA_CORRUPTION = "data_corruption"
    UNKNOWN = "unknown"


@dataclass
class ErrorContext:
    """Context information for error handling and recovery."""
    error: Exception
    category: ErrorCategory
    severity: ErrorSeverity
    operation: str
    connection_id: Optional[int] = None
    gmail_message_id: Optional[str] = None
    gmail_thread_id: Optional[str] = None
    attempt_count: int = 1
    max_attempts: int = 3
    backoff_multiplier: float = 2.0
    timestamp: datetime = field(default_factory=datetime.utcnow)
    metadata: Dict[str, Any] = field(default_factory=dict)
    recovery_actions: List[str] = field(default_factory=list)


@dataclass
class RetryConfig:
    """Configuration for retry behavior."""
    max_attempts: int = 3
    base_delay: float = 1.0
    max_delay: float = 300.0  # 5 minutes
    backoff_multiplier: float = 2.0
    jitter: bool = True
    exponential_backoff: bool = True
    retry_on: Optional[List[Type[Exception]]] = None
    stop_on: Optional[List[Type[Exception]]] = None


class GmailErrorClassifier:
    """Classifies errors and determines appropriate handling strategies."""

    @staticmethod
    def classify_error(error: Exception, operation: str = "") -> ErrorContext:
        """Classify an error and create error context.

        Args:
            error: The exception that occurred
            operation: The operation that was being performed

        Returns:
            ErrorContext with classification and metadata
        """
        # OAuth2 and authentication errors
        if isinstance(error, RefreshError):
            return ErrorContext(
                error=error,
                category=ErrorCategory.OAUTH_TOKEN,
                severity=ErrorSeverity.HIGH,
                operation=operation,
                recovery_actions=["refresh_token", "re_authenticate"]
            )

        # Gmail API HTTP errors
        if isinstance(error, HttpError):
            status_code = error.resp.status

            if status_code == 401:
                return ErrorContext(
                    error=error,
                    category=ErrorCategory.AUTHENTICATION,
                    severity=ErrorSeverity.HIGH,
                    operation=operation,
                    recovery_actions=["refresh_token", "re_authenticate"]
                )
            elif status_code == 403:
                error_details = error.error_details
                if error_details and "quotaExceeded" in str(error_details):
                    return ErrorContext(
                        error=error,
                        category=ErrorCategory.API_QUOTA,
                        severity=ErrorSeverity.MEDIUM,
                        operation=operation,
                        max_attempts=5,
                        backoff_multiplier=5.0,
                        recovery_actions=["exponential_backoff", "quota_reset_wait"]
                    )
                else:
                    return ErrorContext(
                        error=error,
                        category=ErrorCategory.PERMISSION,
                        severity=ErrorSeverity.HIGH,
                        operation=operation,
                        recovery_actions=["check_scopes", "re_authenticate"]
                    )
            elif status_code == 429:
                return ErrorContext(
                    error=error,
                    category=ErrorCategory.API_RATE_LIMIT,
                    severity=ErrorSeverity.MEDIUM,
                    operation=operation,
                    max_attempts=10,
                    backoff_multiplier=2.0,
                    recovery_actions=["exponential_backoff", "rate_limit_wait"]
                )
            elif status_code >= 500:
                return ErrorContext(
                    error=error,
                    category=ErrorCategory.NETWORK,
                    severity=ErrorSeverity.MEDIUM,
                    operation=operation,
                    max_attempts=5,
                    recovery_actions=["retry_with_backoff", "circuit_breaker"]
                )

        # Network and connectivity errors
        if isinstance(error, (ClientError, ClientConnectorError, asyncpg.NetworkError)):
            return ErrorContext(
                error=error,
                category=ErrorCategory.NETWORK,
                severity=ErrorSeverity.MEDIUM,
                operation=operation,
                max_attempts=5,
                recovery_actions=["retry_with_backoff", "network_check"]
            )

        # Database errors
        if isinstance(error, (SQLAlchemyError, asyncpg.PostgresError)):
            severity = ErrorSeverity.CRITICAL if isinstance(error, OperationalError) else ErrorSeverity.MEDIUM
            recovery_actions = ["database_reconnect", "transaction_rollback"]

            if isinstance(error, SQLTimeoutError):
                recovery_actions.append("reduce_batch_size")

            return ErrorContext(
                error=error,
                category=ErrorCategory.DATABASE,
                severity=severity,
                operation=operation,
                recovery_actions=recovery_actions
            )

        # Redis/Event publishing errors
        if isinstance(error, (RedisError, EventPublishError)):
            return ErrorContext(
                error=error,
                category=ErrorCategory.WEBHOOK,
                severity=ErrorSeverity.LOW,
                operation=operation,
                recovery_actions=["fallback_storage", "delayed_retry"]
            )

        # Default classification
        return ErrorContext(
            error=error,
            category=ErrorCategory.UNKNOWN,
            severity=ErrorSeverity.MEDIUM,
            operation=operation,
            recovery_actions=["basic_retry"]
        )


class CircuitBreaker:
    """Circuit breaker pattern for failing operations."""

    def __init__(
        self,
        failure_threshold: int = 5,
        recovery_timeout: int = 60,
        expected_exception: Type[Exception] = Exception
    ):
        self.failure_threshold = failure_threshold
        self.recovery_timeout = recovery_timeout
        self.expected_exception = expected_exception
        self.failure_count = 0
        self.last_failure_time: Optional[float] = None
        self.state = "CLOSED"  # CLOSED, OPEN, HALF_OPEN

    async def call(self, func: Callable, *args, **kwargs) -> Any:
        """Execute function with circuit breaker protection."""
        if self.state == "OPEN":
            if self._should_attempt_reset():
                self.state = "HALF_OPEN"
            else:
                raise Exception(f"Circuit breaker OPEN - service unavailable")

        try:
            result = await func(*args, **kwargs) if asyncio.iscoroutinefunction(func) else func(*args, **kwargs)
            self._on_success()
            return result
        except self.expected_exception as e:
            self._on_failure()
            raise e

    def _should_attempt_reset(self) -> bool:
        """Check if circuit breaker should attempt to reset."""
        return (
            self.last_failure_time is not None and
            time.time() - self.last_failure_time >= self.recovery_timeout
        )

    def _on_success(self):
        """Handle successful call."""
        self.failure_count = 0
        self.state = "CLOSED"

    def _on_failure(self):
        """Handle failed call."""
        self.failure_count += 1
        self.last_failure_time = time.time()

        if self.failure_count >= self.failure_threshold:
            self.state = "OPEN"


class GmailErrorHandler:
    """Comprehensive error handler for Gmail integration operations."""

    def __init__(self):
        self.circuit_breakers: Dict[str, CircuitBreaker] = {}
        self.error_stats: Dict[str, Dict[str, int]] = {}
        self.alert_thresholds = {
            ErrorSeverity.CRITICAL: 1,
            ErrorSeverity.HIGH: 3,
            ErrorSeverity.MEDIUM: 10,
            ErrorSeverity.LOW: 50
        }
        self.alert_windows = {
            ErrorSeverity.CRITICAL: 60,     # 1 minute
            ErrorSeverity.HIGH: 300,        # 5 minutes
            ErrorSeverity.MEDIUM: 900,      # 15 minutes
            ErrorSeverity.LOW: 3600         # 1 hour
        }

    def get_circuit_breaker(self, operation: str) -> CircuitBreaker:
        """Get or create circuit breaker for operation."""
        if operation not in self.circuit_breakers:
            self.circuit_breakers[operation] = CircuitBreaker()
        return self.circuit_breakers[operation]

    async def handle_with_retry(
        self,
        func: Callable[..., Awaitable[Any]],
        operation: str,
        connection_id: Optional[int] = None,
        retry_config: Optional[RetryConfig] = None,
        *args,
        **kwargs
    ) -> Any:
        """Execute function with comprehensive error handling and retry logic.

        Args:
            func: Async function to execute
            operation: Operation name for logging and circuit breaker
            connection_id: Gmail connection ID (if applicable)
            retry_config: Custom retry configuration
            *args: Function arguments
            **kwargs: Function keyword arguments

        Returns:
            Function result

        Raises:
            Exception: If all retry attempts fail
        """
        config = retry_config or RetryConfig()
        circuit_breaker = self.get_circuit_breaker(operation)

        last_error = None

        for attempt in range(1, config.max_attempts + 1):
            try:
                # Use circuit breaker for protection
                result = await circuit_breaker.call(func, *args, **kwargs)

                # Log successful recovery if this wasn't the first attempt
                if attempt > 1:
                    logger.info(f"Operation '{operation}' recovered after {attempt} attempts")

                return result

            except Exception as error:
                last_error = error

                # Classify the error
                error_context = GmailErrorClassifier.classify_error(error, operation)
                error_context.connection_id = connection_id
                error_context.attempt_count = attempt
                error_context.max_attempts = config.max_attempts

                # Log the error
                await self._log_error(error_context)

                # Update error statistics
                self._update_error_stats(error_context)

                # Check if we should stop retrying
                if not self._should_retry(error, config, attempt):
                    break

                # Calculate backoff delay
                delay = self._calculate_backoff(config, attempt, error_context)

                # Apply specific recovery actions
                await self._apply_recovery_actions(error_context)

                # Check alert thresholds
                await self._check_alert_thresholds(error_context)

                # Wait before retry (unless this is the last attempt)
                if attempt < config.max_attempts:
                    logger.info(f"Retrying operation '{operation}' in {delay:.2f}s (attempt {attempt + 1}/{config.max_attempts})")
                    await asyncio.sleep(delay)

        # All attempts failed
        if last_error:
            final_context = GmailErrorClassifier.classify_error(last_error, operation)
            final_context.connection_id = connection_id
            final_context.attempt_count = config.max_attempts
            final_context.severity = ErrorSeverity.CRITICAL  # Escalate to critical

            await self._log_error(final_context, is_final=True)
            await self._trigger_critical_alert(final_context)

            raise last_error

        raise Exception(f"Operation '{operation}' failed with unknown error")

    def _should_retry(self, error: Exception, config: RetryConfig, attempt: int) -> bool:
        """Determine if operation should be retried."""
        # Check if we've reached max attempts
        if attempt >= config.max_attempts:
            return False

        # Check stop_on conditions
        if config.stop_on and any(isinstance(error, exc_type) for exc_type in config.stop_on):
            return False

        # Check retry_on conditions
        if config.retry_on and not any(isinstance(error, exc_type) for exc_type in config.retry_on):
            return False

        # Check for specific non-retryable errors
        if isinstance(error, HttpError):
            status_code = error.resp.status
            # Don't retry for client errors that won't be fixed by retrying
            if status_code in [400, 404, 410]:  # Bad Request, Not Found, Gone
                return False

        return True

    def _calculate_backoff(
        self,
        config: RetryConfig,
        attempt: int,
        error_context: ErrorContext
    ) -> float:
        """Calculate backoff delay for retry."""
        if config.exponential_backoff:
            # Exponential backoff with optional jitter
            delay = config.base_delay * (config.backoff_multiplier ** (attempt - 1))

            # Apply category-specific multipliers
            if error_context.category == ErrorCategory.API_QUOTA:
                delay *= 5  # Longer delays for quota errors
            elif error_context.category == ErrorCategory.API_RATE_LIMIT:
                delay *= 2  # Moderate delays for rate limiting
        else:
            # Linear backoff
            delay = config.base_delay * attempt

        # Apply maximum delay limit
        delay = min(delay, config.max_delay)

        # Add jitter to prevent thundering herd
        if config.jitter:
            import random
            delay *= (0.5 + random.random() * 0.5)

        return delay

    async def _apply_recovery_actions(self, error_context: ErrorContext) -> None:
        """Apply specific recovery actions based on error type."""
        for action in error_context.recovery_actions:
            try:
                if action == "refresh_token":
                    await self._refresh_oauth_token(error_context)
                elif action == "database_reconnect":
                    await self._reconnect_database(error_context)
                elif action == "transaction_rollback":
                    await self._rollback_transaction(error_context)
                elif action == "clear_cache":
                    await self._clear_cache(error_context)
                elif action == "reduce_batch_size":
                    await self._reduce_batch_size(error_context)
                # Additional actions can be added here
            except Exception as recovery_error:
                logger.error(f"Recovery action '{action}' failed: {recovery_error}")

    async def _refresh_oauth_token(self, error_context: ErrorContext) -> None:
        """Attempt to refresh OAuth2 token."""
        if not error_context.connection_id:
            return

        try:
            from .auth import GmailAuthenticator
            from ...models.gmail_connection import GmailConnection
            from sqlalchemy import select

            async with db_manager.get_session() as session:
                result = await session.execute(
                    select(GmailConnection).where(GmailConnection.id == error_context.connection_id)
                )
                connection = result.scalar_one_or_none()

                if connection and connection.encrypted_credentials:
                    authenticator = GmailAuthenticator({})
                    credentials = await authenticator.decrypt_credentials(connection.encrypted_credentials)

                    if credentials.refresh_token:
                        refreshed = await authenticator.refresh_credentials(credentials)
                        # Update stored credentials
                        connection.encrypted_credentials = await authenticator.encrypt_credentials(refreshed)
                        await session.commit()

                        logger.info(f"Successfully refreshed OAuth token for connection {error_context.connection_id}")
                    else:
                        logger.error(f"No refresh token available for connection {error_context.connection_id}")

        except Exception as e:
            logger.error(f"Failed to refresh OAuth token: {e}")

    async def _reconnect_database(self, error_context: ErrorContext) -> None:
        """Attempt to reconnect to database."""
        try:
            await db_manager.close_all()
            # Force new connection on next use
            logger.info("Database reconnection requested")
        except Exception as e:
            logger.error(f"Database reconnection failed: {e}")

    async def _rollback_transaction(self, error_context: ErrorContext) -> None:
        """Rollback any pending database transactions."""
        try:
            # This would need to be implemented based on your transaction management
            logger.info("Transaction rollback requested")
        except Exception as e:
            logger.error(f"Transaction rollback failed: {e}")

    async def _clear_cache(self, error_context: ErrorContext) -> None:
        """Clear relevant caches."""
        try:
            # Clear any connection-specific caches
            logger.info("Cache clear requested")
        except Exception as e:
            logger.error(f"Cache clear failed: {e}")

    async def _reduce_batch_size(self, error_context: ErrorContext) -> None:
        """Signal to reduce batch size for subsequent operations."""
        try:
            # Store hint to reduce batch size
            error_context.metadata["reduce_batch_size"] = True
            logger.info("Batch size reduction requested")
        except Exception as e:
            logger.error(f"Batch size reduction signal failed: {e}")

    async def _log_error(self, error_context: ErrorContext, is_final: bool = False) -> None:
        """Log error with appropriate level and details."""
        log_data = {
            "operation": error_context.operation,
            "category": error_context.category.value,
            "severity": error_context.severity.value,
            "attempt": error_context.attempt_count,
            "max_attempts": error_context.max_attempts,
            "connection_id": error_context.connection_id,
            "gmail_message_id": error_context.gmail_message_id,
            "error_type": type(error_context.error).__name__,
            "error_message": str(error_context.error),
            "recovery_actions": error_context.recovery_actions,
            "is_final": is_final
        }

        if error_context.severity in [ErrorSeverity.CRITICAL, ErrorSeverity.HIGH]:
            logger.error(f"Gmail operation error: {json.dumps(log_data, default=str)}")
            # Also log the full stack trace for serious errors
            logger.error(f"Stack trace: {traceback.format_exc()}")
        elif error_context.severity == ErrorSeverity.MEDIUM:
            logger.warning(f"Gmail operation error: {json.dumps(log_data, default=str)}")
        else:
            logger.info(f"Gmail operation error: {json.dumps(log_data, default=str)}")

    def _update_error_stats(self, error_context: ErrorContext) -> None:
        """Update error statistics for monitoring."""
        category = error_context.category.value
        severity = error_context.severity.value

        if category not in self.error_stats:
            self.error_stats[category] = {}

        if severity not in self.error_stats[category]:
            self.error_stats[category][severity] = 0

        self.error_stats[category][severity] += 1

    async def _check_alert_thresholds(self, error_context: ErrorContext) -> None:
        """Check if error thresholds have been exceeded and trigger alerts."""
        severity = error_context.severity
        category = error_context.category.value

        if category in self.error_stats and severity.value in self.error_stats[category]:
            error_count = self.error_stats[category][severity.value]
            threshold = self.alert_thresholds[severity]

            if error_count >= threshold:
                await self._trigger_alert(error_context, error_count, threshold)

    async def _trigger_alert(
        self,
        error_context: ErrorContext,
        error_count: int,
        threshold: int
    ) -> None:
        """Trigger appropriate alert based on error severity."""
        alert_data = {
            "alert_type": "error_threshold_exceeded",
            "category": error_context.category.value,
            "severity": error_context.severity.value,
            "error_count": error_count,
            "threshold": threshold,
            "operation": error_context.operation,
            "connection_id": error_context.connection_id,
            "timestamp": datetime.utcnow().isoformat()
        }

        logger.critical(f"Gmail error threshold exceeded: {json.dumps(alert_data, default=str)}")

        # Here you would integrate with your alerting system
        # (PagerDuty, Slack, email, etc.)
        # await self._send_alert_notification(alert_data)

    async def _trigger_critical_alert(self, error_context: ErrorContext) -> None:
        """Trigger critical alert for operation failure."""
        alert_data = {
            "alert_type": "operation_failure",
            "category": error_context.category.value,
            "severity": "critical",
            "operation": error_context.operation,
            "connection_id": error_context.connection_id,
            "error_type": type(error_context.error).__name__,
            "error_message": str(error_context.error),
            "attempt_count": error_context.attempt_count,
            "timestamp": datetime.utcnow().isoformat()
        }

        logger.critical(f"Gmail operation failed permanently: {json.dumps(alert_data, default=str)}")

        # Here you would integrate with your critical alerting system
        # await self._send_critical_alert(alert_data)

    def get_error_statistics(self) -> Dict[str, Any]:
        """Get current error statistics."""
        return {
            "error_stats": self.error_stats,
            "circuit_breakers": {
                name: {
                    "state": cb.state,
                    "failure_count": cb.failure_count,
                    "last_failure_time": cb.last_failure_time
                }
                for name, cb in self.circuit_breakers.items()
            }
        }

    def reset_error_statistics(self) -> None:
        """Reset error statistics (useful for testing or periodic cleanup)."""
        self.error_stats.clear()
        for cb in self.circuit_breakers.values():
            cb.failure_count = 0
            cb.last_failure_time = None
            cb.state = "CLOSED"


# Global error handler instance
gmail_error_handler = GmailErrorHandler()


def with_error_handling(
    operation: str,
    connection_id: Optional[int] = None,
    retry_config: Optional[RetryConfig] = None
):
    """Decorator for adding error handling to async functions.

    Args:
        operation: Operation name for logging and monitoring
        connection_id: Gmail connection ID (if applicable)
        retry_config: Custom retry configuration
    """
    def decorator(func: Callable[..., Awaitable[Any]]):
        async def wrapper(*args, **kwargs):
            return await gmail_error_handler.handle_with_retry(
                func, operation, connection_id, retry_config, *args, **kwargs
            )
        return wrapper
    return decorator