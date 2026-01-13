"""Gmail synchronization operations for historical and incremental sync."""

import asyncio
import logging
import uuid
from datetime import datetime, timedelta
from typing import Optional, Dict, Any, List, AsyncGenerator, Tuple
from pathlib import Path
import base64
import email
from email.header import decode_header

from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, and_

from .client import GmailClient
from .auth import GmailAuthenticator
from ...models.gmail_connection import GmailConnection
from ...models.email_message import EmailMessage, EmailThread, EmailAttachment
from ...events.publishers import EmailEventPublisher, SyncEventPublisher
from ...storage.database import db_manager


logger = logging.getLogger(__name__)


class GmailSyncManager:
    """Manages Gmail synchronization operations."""

    def __init__(
        self,
        connection_id: int,
        gmail_client: GmailClient,
        email_publisher: EmailEventPublisher,
        sync_publisher: SyncEventPublisher,
        attachment_base_path: Optional[str] = None
    ):
        """Initialize sync manager.

        Args:
            connection_id: Gmail connection ID
            gmail_client: Authenticated Gmail client
            email_publisher: Email event publisher
            sync_publisher: Sync event publisher
            attachment_base_path: Base path for downloading attachments
        """
        self.connection_id = connection_id
        self.gmail_client = gmail_client
        self.email_publisher = email_publisher
        self.sync_publisher = sync_publisher
        self.attachment_base_path = Path(attachment_base_path or "./attachments")
        self.sync_session_id = str(uuid.uuid4())

    async def sync_historical_emails(
        self,
        max_results: int = 1000,
        query: str = "",
        batch_size: int = 20,
        start_date: Optional[datetime] = None,
        end_date: Optional[datetime] = None
    ) -> Dict[str, Any]:
        """Perform historical email synchronization.

        Args:
            max_results: Maximum emails to sync
            query: Gmail search query
            batch_size: Number of emails per batch
            start_date: Start date for sync
            end_date: End date for sync

        Returns:
            Sync summary with statistics
        """
        start_time = datetime.utcnow()

        # Build query with date filters
        full_query = query
        if start_date:
            full_query += f" after:{start_date.strftime('%Y/%m/%d')}"
        if end_date:
            full_query += f" before:{end_date.strftime('%Y/%m/%d')}"

        logger.info(f"Starting historical sync for connection {self.connection_id}")
        logger.info(f"Query: {full_query}, Max results: {max_results}")

        await self.sync_publisher.publish_sync_progress(
            sync_session_id=self.sync_session_id,
            connection_id=self.connection_id,
            sync_type="historical",
            processed_items=0,
            current_batch=0,
            status="starting",
            current_operation="Initializing historical sync"
        )

        try:
            # Get list of all messages matching query
            messages_processed = 0
            threads_processed = 0
            attachments_downloaded = 0
            errors = []

            async for batch_results in self._get_message_batches(
                full_query, max_results, batch_size
            ):
                batch_num = (messages_processed // batch_size) + 1

                await self.sync_publisher.publish_sync_progress(
                    sync_session_id=self.sync_session_id,
                    connection_id=self.connection_id,
                    sync_type="historical",
                    processed_items=messages_processed,
                    current_batch=batch_num,
                    status="in_progress",
                    current_operation=f"Processing batch {batch_num}",
                    total_items=max_results
                )

                # Process messages in batch
                batch_messages = 0
                batch_threads = set()
                batch_attachments = 0

                for message_ref in batch_results.get('messages', []):
                    try:
                        result = await self._process_single_message(message_ref['id'])
                        if result:
                            batch_messages += 1
                            batch_threads.add(result['thread_id'])
                            batch_attachments += result['attachment_count']

                    except Exception as e:
                        logger.error(f"Failed to process message {message_ref['id']}: {e}")
                        errors.append(f"Message {message_ref['id']}: {str(e)}")

                messages_processed += batch_messages
                threads_processed += len(batch_threads)
                attachments_downloaded += batch_attachments

                # Rate limiting - small delay between batches
                await asyncio.sleep(0.1)

                # Check if we've reached max_results
                if messages_processed >= max_results:
                    break

            # Calculate performance metrics
            duration = (datetime.utcnow() - start_time).total_seconds()
            items_per_minute = (messages_processed / duration * 60) if duration > 0 else 0

            # Publish completion event
            await self.sync_publisher.publish_sync_completed(
                sync_session_id=self.sync_session_id,
                connection_id=self.connection_id,
                sync_type="historical",
                total_messages=messages_processed,
                total_threads=threads_processed,
                total_attachments=attachments_downloaded,
                duration_seconds=duration,
                success=len(errors) == 0,
                failed_messages=len(errors),
                error_summary="; ".join(errors[:5]) if errors else None
            )

            logger.info(f"Historical sync completed: {messages_processed} messages, "
                       f"{threads_processed} threads, {attachments_downloaded} attachments "
                       f"in {duration:.2f}s ({items_per_minute:.1f} items/min)")

            return {
                'success': len(errors) == 0,
                'messages_processed': messages_processed,
                'threads_processed': threads_processed,
                'attachments_downloaded': attachments_downloaded,
                'duration_seconds': duration,
                'items_per_minute': items_per_minute,
                'errors': errors
            }

        except Exception as e:
            logger.error(f"Historical sync failed: {e}")
            await self.sync_publisher.publish_sync_completed(
                sync_session_id=self.sync_session_id,
                connection_id=self.connection_id,
                sync_type="historical",
                total_messages=0,
                total_threads=0,
                duration_seconds=(datetime.utcnow() - start_time).total_seconds(),
                success=False,
                error_summary=str(e)
            )
            raise

    async def _get_message_batches(
        self,
        query: str,
        max_results: int,
        batch_size: int
    ) -> AsyncGenerator[Dict[str, Any], None]:
        """Get messages in batches from Gmail API.

        Args:
            query: Gmail search query
            max_results: Maximum total results
            batch_size: Results per batch

        Yields:
            Batch of message references
        """
        page_token = None
        total_fetched = 0

        while total_fetched < max_results:
            # Calculate how many to fetch in this batch
            remaining = max_results - total_fetched
            current_batch_size = min(batch_size, remaining)

            try:
                result = await self.gmail_client.list_messages(
                    query=query,
                    max_results=current_batch_size,
                    page_token=page_token
                )

                if 'messages' not in result:
                    break

                yield result
                total_fetched += len(result.get('messages', []))

                # Get next page token
                page_token = result.get('nextPageToken')
                if not page_token:
                    break

            except Exception as e:
                logger.error(f"Failed to fetch message batch: {e}")
                break

    async def _process_single_message(self, message_id: str) -> Optional[Dict[str, Any]]:
        """Process a single Gmail message.

        Args:
            message_id: Gmail message ID

        Returns:
            Processing result summary
        """
        async with db_manager.get_session() as session:
            # Check if message already exists
            existing = await session.execute(
                select(EmailMessage).where(EmailMessage.gmail_message_id == message_id)
            )
            if existing.scalar_one_or_none():
                logger.debug(f"Message {message_id} already exists, skipping")
                return None

            # Fetch full message from Gmail
            try:
                message_data = await self.gmail_client.get_message(message_id)
            except Exception as e:
                logger.error(f"Failed to fetch message {message_id}: {e}")
                return None

            # Extract message metadata
            thread_id = message_data['threadId']
            headers = self._extract_headers(message_data['payload'].get('headers', []))

            # Get or create thread
            email_thread = await self._get_or_create_thread(session, thread_id, headers)

            # Extract message content
            content = self._extract_message_content(message_data['payload'])

            # Create email message record
            email_message = EmailMessage(
                gmail_message_id=message_id,
                thread_id=email_thread.id,
                subject=headers.get('subject'),
                from_email=headers.get('from_email', ''),
                from_name=headers.get('from_name'),
                to_emails=self._parse_email_addresses(headers.get('to', '')),
                cc_emails=self._parse_email_addresses(headers.get('cc', '')) if headers.get('cc') else None,
                bcc_emails=self._parse_email_addresses(headers.get('bcc', '')) if headers.get('bcc') else None,
                body_text=content.get('text'),
                body_html=content.get('html'),
                snippet=message_data.get('snippet'),
                gmail_labels=message_data.get('labelIds', []),
                internal_date=datetime.fromtimestamp(int(message_data['internalDate']) / 1000),
                is_unread='UNREAD' in message_data.get('labelIds', []),
                size_estimate=message_data.get('sizeEstimate'),
                has_attachments=content['has_attachments'],
                attachment_count=content['attachment_count'],
                processing_status='processing'
            )

            session.add(email_message)
            await session.commit()

            # Process attachments if any
            attachment_count = 0
            if content['attachments']:
                attachment_count = await self._process_attachments(
                    session, email_message, content['attachments']
                )

            # Update thread statistics
            email_thread.message_count = (email_thread.message_count or 0) + 1
            if not email_thread.first_message_date or email_message.internal_date < email_thread.first_message_date:
                email_thread.first_message_date = email_message.internal_date
            if not email_thread.last_message_date or email_message.internal_date > email_thread.last_message_date:
                email_thread.last_message_date = email_message.internal_date

            # Mark message as processed and publish event
            email_message.processing_status = 'completed'
            email_message.event_published_at = datetime.utcnow()
            await session.commit()

            # Publish email ingested event
            await self.email_publisher.publish_email_ingested(
                gmail_message_id=message_id,
                gmail_thread_id=thread_id,
                connection_id=self.connection_id,
                subject=email_message.subject,
                from_email=email_message.from_email,
                from_name=email_message.from_name,
                to_emails=email_message.to_emails,
                internal_date=email_message.internal_date,
                cc_emails=email_message.cc_emails,
                bcc_emails=email_message.bcc_emails,
                has_body_text=bool(email_message.body_text),
                has_body_html=bool(email_message.body_html),
                has_attachments=email_message.has_attachments,
                attachment_count=attachment_count,
                gmail_labels=email_message.gmail_labels,
                is_unread=email_message.is_unread,
                size_estimate=email_message.size_estimate,
                thread_message_count=email_thread.message_count,
                is_first_message=email_thread.message_count == 1
            )

            return {
                'thread_id': thread_id,
                'attachment_count': attachment_count,
                'success': True
            }

    async def _get_or_create_thread(
        self,
        session: AsyncSession,
        gmail_thread_id: str,
        headers: Dict[str, Any]
    ) -> EmailThread:
        """Get existing thread or create new one.

        Args:
            session: Database session
            gmail_thread_id: Gmail thread ID
            headers: Message headers

        Returns:
            EmailThread instance
        """
        # Check if thread exists
        existing = await session.execute(
            select(EmailThread).where(
                and_(
                    EmailThread.gmail_thread_id == gmail_thread_id,
                    EmailThread.connection_id == self.connection_id
                )
            )
        )
        thread = existing.scalar_one_or_none()

        if not thread:
            # Create new thread
            thread = EmailThread(
                gmail_thread_id=gmail_thread_id,
                connection_id=self.connection_id,
                subject=headers.get('subject'),
                participant_emails=[headers.get('from_email', '')],
                message_count=0,
                processing_status='processing'
            )
            session.add(thread)
            await session.commit()
            await session.refresh(thread)

        return thread

    def _extract_headers(self, headers: List[Dict[str, str]]) -> Dict[str, Any]:
        """Extract and normalize email headers.

        Args:
            headers: Raw Gmail headers

        Returns:
            Normalized header dictionary
        """
        header_dict = {}

        for header in headers:
            name = header['name'].lower()
            value = header['value']

            if name == 'from':
                # Parse from field
                parsed = email.utils.parseaddr(value)
                header_dict['from_email'] = parsed[1]
                header_dict['from_name'] = parsed[0] if parsed[0] else None
            elif name in ['to', 'cc', 'bcc', 'subject', 'date']:
                header_dict[name] = value

        return header_dict

    def _parse_email_addresses(self, address_string: str) -> List[str]:
        """Parse comma-separated email addresses.

        Args:
            address_string: String of email addresses

        Returns:
            List of email addresses
        """
        if not address_string:
            return []

        addresses = []
        for addr in email.utils.getaddresses([address_string]):
            if addr[1]:  # Email address exists
                addresses.append(addr[1])

        return addresses

    def _extract_message_content(self, payload: Dict[str, Any]) -> Dict[str, Any]:
        """Extract text and HTML content from message payload.

        Args:
            payload: Gmail message payload

        Returns:
            Content dictionary with text, html, attachments
        """
        content = {
            'text': None,
            'html': None,
            'has_attachments': False,
            'attachment_count': 0,
            'attachments': []
        }

        def _extract_parts(part: Dict[str, Any]):
            """Recursively extract parts from message."""
            mime_type = part.get('mimeType', '')

            if 'parts' in part:
                # Multipart message, recurse into parts
                for subpart in part['parts']:
                    _extract_parts(subpart)
            else:
                # Single part
                if mime_type == 'text/plain' and 'data' in part.get('body', {}):
                    # Plain text content
                    data = part['body']['data']
                    content['text'] = base64.urlsafe_b64decode(data).decode('utf-8')
                elif mime_type == 'text/html' and 'data' in part.get('body', {}):
                    # HTML content
                    data = part['body']['data']
                    content['html'] = base64.urlsafe_b64decode(data).decode('utf-8')
                elif 'filename' in part and part['filename']:
                    # Attachment
                    content['has_attachments'] = True
                    content['attachment_count'] += 1
                    content['attachments'].append({
                        'filename': part['filename'],
                        'mime_type': mime_type,
                        'attachment_id': part['body'].get('attachmentId'),
                        'size': part['body'].get('size', 0)
                    })

        _extract_parts(payload)
        return content

    async def _process_attachments(
        self,
        session: AsyncSession,
        email_message: EmailMessage,
        attachments: List[Dict[str, Any]]
    ) -> int:
        """Process and download email attachments.

        Args:
            session: Database session
            email_message: Parent email message
            attachments: List of attachment metadata

        Returns:
            Number of attachments processed
        """
        processed_count = 0

        for attachment_info in attachments:
            try:
                # Create attachment record
                attachment = EmailAttachment(
                    message_id=email_message.id,
                    gmail_attachment_id=attachment_info['attachment_id'],
                    filename=attachment_info['filename'],
                    mime_type=attachment_info['mime_type'],
                    size_bytes=attachment_info['size'],
                    download_status='pending'
                )

                session.add(attachment)
                await session.commit()
                await session.refresh(attachment)

                # Download attachment if not too large (limit: 10MB)
                if attachment.size_bytes <= 10 * 1024 * 1024:
                    try:
                        attachment.download_status = 'downloading'
                        await session.commit()

                        # Download attachment data
                        attachment_data = await self.gmail_client.download_attachment(
                            email_message.gmail_message_id,
                            attachment.gmail_attachment_id
                        )

                        # Save to file
                        file_path = self._get_attachment_file_path(attachment)
                        file_path.parent.mkdir(parents=True, exist_ok=True)

                        with open(file_path, 'wb') as f:
                            f.write(attachment_data)

                        attachment.file_path = str(file_path)
                        attachment.download_status = 'completed'
                        attachment.downloaded_at = datetime.utcnow()

                        # Publish attachment event
                        await self.email_publisher.publish_attachment_ingested(
                            gmail_attachment_id=attachment.gmail_attachment_id,
                            gmail_message_id=email_message.gmail_message_id,
                            gmail_thread_id=email_message.thread.gmail_thread_id,
                            connection_id=self.connection_id,
                            filename=attachment.filename,
                            mime_type=attachment.mime_type,
                            size_bytes=attachment.size_bytes,
                            file_path=str(file_path)
                        )

                    except Exception as e:
                        logger.error(f"Failed to download attachment {attachment.filename}: {e}")
                        attachment.download_status = 'failed'
                else:
                    # Skip large attachments
                    attachment.download_status = 'skipped_large'

                await session.commit()
                processed_count += 1

            except Exception as e:
                logger.error(f"Failed to process attachment {attachment_info['filename']}: {e}")

        return processed_count

    def _get_attachment_file_path(self, attachment: EmailAttachment) -> Path:
        """Get file path for attachment storage.

        Args:
            attachment: Email attachment record

        Returns:
            Path object for attachment file
        """
        # Organize by connection and date
        date_str = attachment.created_at.strftime('%Y/%m/%d')
        safe_filename = "".join(c for c in attachment.filename if c.isalnum() or c in '.-_')

        return self.attachment_base_path / str(self.connection_id) / date_str / f"{attachment.id}_{safe_filename}"


async def create_sync_manager(connection_id: int) -> GmailSyncManager:
    """Create a sync manager for a Gmail connection.

    Args:
        connection_id: Gmail connection ID

    Returns:
        Configured GmailSyncManager
    """
    # Get connection from database
    async with db_manager.get_session() as session:
        result = await session.execute(
            select(GmailConnection).where(GmailConnection.id == connection_id)
        )
        connection = result.scalar_one_or_none()

        if not connection:
            raise ValueError(f"Gmail connection {connection_id} not found")

        if not connection.is_active:
            raise ValueError(f"Gmail connection {connection_id} is not active")

    # Decrypt credentials
    authenticator = GmailAuthenticator({})  # Empty config for decryption only
    credentials = await authenticator.decrypt_credentials(connection.encrypted_credentials)

    # Validate and refresh if needed
    if not await authenticator.validate_credentials(credentials):
        raise ValueError(f"Gmail connection {connection_id} has invalid credentials")

    # Create Gmail client
    gmail_client = GmailClient(credentials)

    # Create event publishers
    email_publisher = EmailEventPublisher()
    sync_publisher = SyncEventPublisher()

    return GmailSyncManager(
        connection_id=connection_id,
        gmail_client=gmail_client,
        email_publisher=email_publisher,
        sync_publisher=sync_publisher
    )