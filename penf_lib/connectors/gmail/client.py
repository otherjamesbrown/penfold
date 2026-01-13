"""Gmail API client wrapper with rate limiting and error handling."""

from typing import Optional, Dict, Any, List, AsyncGenerator
import asyncio
from datetime import datetime, timedelta
import logging

from googleapiclient.discovery import build
from google.oauth2.credentials import Credentials
from googleapiclient.errors import HttpError

from .auth import GmailAuthenticator
from .error_handling import with_error_handling, RetryConfig, ErrorCategory


logger = logging.getLogger(__name__)


class GmailRateLimiter:
    """Rate limiter for Gmail API to respect quota limits."""

    def __init__(self, calls_per_second: int = 10):
        """Initialize rate limiter.

        Args:
            calls_per_second: Maximum API calls per second
        """
        self.calls_per_second = calls_per_second
        self.call_times: List[float] = []
        self._lock = asyncio.Lock()

    async def wait_if_needed(self) -> None:
        """Wait if necessary to respect rate limits."""
        async with self._lock:
            now = datetime.utcnow().timestamp()

            # Remove calls older than 1 second
            self.call_times = [t for t in self.call_times if now - t < 1.0]

            if len(self.call_times) >= self.calls_per_second:
                # Need to wait
                oldest_relevant = min(self.call_times)
                wait_time = 1.0 - (now - oldest_relevant)
                if wait_time > 0:
                    await asyncio.sleep(wait_time)

            self.call_times.append(now)


class GmailClient:
    """Async Gmail API client with rate limiting and error handling."""

    def __init__(
        self,
        credentials: Credentials,
        rate_limiter: Optional[GmailRateLimiter] = None,
        connection_id: Optional[int] = None
    ) -> None:
        """Initialize Gmail client.

        Args:
            credentials: Authenticated OAuth2 credentials
            rate_limiter: Rate limiter instance
            connection_id: Gmail connection ID for error tracking
        """
        self.credentials = credentials
        self.rate_limiter = rate_limiter or GmailRateLimiter()
        self.connection_id = connection_id
        self._service = None

        # Configure retry settings for different operations
        self.api_retry_config = RetryConfig(
            max_attempts=5,
            base_delay=1.0,
            max_delay=60.0,
            backoff_multiplier=2.0,
            exponential_backoff=True
        )

    @property
    def service(self):
        """Get Gmail API service instance."""
        if self._service is None:
            self._service = build('gmail', 'v1', credentials=self.credentials)
        return self._service

    async def _make_api_call(self, func, operation_name: str, *args, **kwargs) -> Any:
        """Make rate-limited API call with comprehensive error handling.

        Args:
            func: Gmail API function to call
            operation_name: Name of the operation for error tracking
            *args: Function arguments
            **kwargs: Function keyword arguments

        Returns:
            API response
        """
        @with_error_handling(
            operation=f"gmail_api_{operation_name}",
            connection_id=self.connection_id,
            retry_config=self.api_retry_config
        )
        async def _api_call_with_rate_limit():
            await self.rate_limiter.wait_if_needed()

            # Execute in thread pool since Gmail API is synchronous
            loop = asyncio.get_event_loop()
            return await loop.run_in_executor(None, lambda: func(*args, **kwargs))

        return await _api_call_with_rate_limit()

    async def get_profile(self) -> Dict[str, Any]:
        """Get user's Gmail profile information.

        Returns:
            Gmail profile data
        """
        return await self._make_api_call(
            self.service.users().getProfile(userId='me').execute,
            'get_profile'
        )

    async def list_messages(
        self,
        query: str = "",
        max_results: int = 100,
        page_token: Optional[str] = None
    ) -> Dict[str, Any]:
        """List Gmail messages matching query.

        Args:
            query: Gmail search query
            max_results: Maximum messages to return
            page_token: Pagination token

        Returns:
            List of message metadata
        """
        return await self._make_api_call(
            self.service.users().messages().list(
                userId='me',
                q=query,
                maxResults=max_results,
                pageToken=page_token
            ).execute,
            'list_messages'
        )

    async def get_message(
        self,
        message_id: str,
        format: str = 'full',
        metadata_headers: Optional[List[str]] = None
    ) -> Dict[str, Any]:
        """Get full Gmail message by ID.

        Args:
            message_id: Gmail message ID
            format: Message format (full, metadata, minimal, raw)
            metadata_headers: Headers to include in metadata format

        Returns:
            Complete message data
        """
        return await self._make_api_call(
            self.service.users().messages().get(
                userId='me',
                id=message_id,
                format=format,
                metadataHeaders=metadata_headers
            ).execute,
            'get_message'
        )

    async def get_thread(self, thread_id: str) -> Dict[str, Any]:
        """Get Gmail thread by ID.

        Args:
            thread_id: Gmail thread ID

        Returns:
            Complete thread data with all messages
        """
        return await self._make_api_call(
            self.service.users().threads().get(
                userId='me',
                id=thread_id
            ).execute,
            'get_thread'
        )

    async def download_attachment(
        self,
        message_id: str,
        attachment_id: str
    ) -> bytes:
        """Download message attachment.

        Args:
            message_id: Gmail message ID
            attachment_id: Attachment ID within message

        Returns:
            Attachment data
        """
        attachment = await self._make_api_call(
            self.service.users().messages().attachments().get(
                userId='me',
                messageId=message_id,
                id=attachment_id
            ).execute,
            'download_attachment'
        )

        import base64
        return base64.urlsafe_b64decode(attachment['data'])

    async def watch_mailbox(
        self,
        topic_name: str,
        label_ids: Optional[List[str]] = None,
        label_filter_action: str = 'include'
    ) -> Dict[str, Any]:
        """Set up push notifications for mailbox changes.

        Args:
            topic_name: Cloud Pub/Sub topic name
            label_ids: Labels to watch
            label_filter_action: 'include' or 'exclude'

        Returns:
            Watch response with expiration
        """
        request = {
            'topicName': topic_name
        }

        if label_ids:
            request['labelFilterAction'] = label_filter_action
            request['labelIds'] = label_ids

        return await self._make_api_call(
            self.service.users().watch(
                userId='me',
                body=request
            ).execute,
            'watch_mailbox'
        )

    async def stop_watch(self) -> None:
        """Stop push notifications for mailbox."""
        await self._make_api_call(
            self.service.users().stop(userId='me').execute,
            'stop_watch'
        )

    async def get_history(
        self,
        start_history_id: str,
        max_results: int = 100,
        page_token: Optional[str] = None,
        label_id: Optional[str] = None,
        history_types: Optional[List[str]] = None
    ) -> Dict[str, Any]:
        """Get mailbox history since given history ID.

        Args:
            start_history_id: History ID to start from
            max_results: Maximum history records
            page_token: Pagination token
            label_id: Filter by label
            history_types: Types of history to include

        Returns:
            History data
        """
        request = {
            'startHistoryId': start_history_id,
            'maxResults': max_results
        }

        if page_token:
            request['pageToken'] = page_token
        if label_id:
            request['labelId'] = label_id
        if history_types:
            request['historyTypes'] = history_types

        return await self._make_api_call(
            self.service.users().history().list(
                userId='me',
                **request
            ).execute,
            'get_history'
        )