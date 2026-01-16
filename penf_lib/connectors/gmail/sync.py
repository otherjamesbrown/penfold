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
from .metadata import GmailMetadataExtractor, ExtractedMetadata
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
        self.metadata_extractor = GmailMetadataExtractor()

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

                # Process messages in batch with optimized database operations
                message_ids = [msg['id'] for msg in batch_results.get('messages', [])]
                batch_result = await self._process_message_batch(message_ids)

                batch_messages = batch_result['processed_count']
                batch_threads = batch_result['thread_ids']
                batch_attachments = batch_result['attachment_count']
                errors.extend(batch_result['errors'])

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

    async def _process_message_batch(
        self,
        message_ids: List[str]
    ) -> Dict[str, Any]:
        """Process a batch of messages with optimized database operations.

        Reduces N+1 queries by:
        1. Batch checking for existing messages
        2. Batch fetching Gmail messages
        3. Batch creating/finding threads
        4. Batch inserting messages

        Args:
            message_ids: List of Gmail message IDs to process

        Returns:
            Batch processing result summary
        """
        result = {
            'processed_count': 0,
            'thread_ids': set(),
            'attachment_count': 0,
            'errors': []
        }

        if not message_ids:
            return result

        async with db_manager.get_session() as session:
            try:
                # Step 1: Batch check for existing messages (single query)
                existing_query = select(EmailMessage.gmail_message_id).where(
                    EmailMessage.gmail_message_id.in_(message_ids)
                )
                existing_result = await session.execute(existing_query)
                existing_ids = {row[0] for row in existing_result}

                # Filter to only new messages
                new_message_ids = [mid for mid in message_ids if mid not in existing_ids]

                if not new_message_ids:
                    logger.debug(f"All {len(message_ids)} messages already exist, skipping batch")
                    return result

                logger.debug(f"Processing batch: {len(new_message_ids)} new, {len(existing_ids)} existing")

                # Step 2: Fetch all new messages from Gmail in parallel
                fetch_tasks = [
                    self.gmail_client.get_message(mid)
                    for mid in new_message_ids
                ]
                gmail_messages = await asyncio.gather(*fetch_tasks, return_exceptions=True)

                # Step 3: Collect unique thread IDs for batch lookup
                thread_ids_needed = set()
                valid_messages = []

                for mid, gmail_msg in zip(new_message_ids, gmail_messages):
                    if isinstance(gmail_msg, Exception):
                        logger.error(f"Failed to fetch message {mid}: {gmail_msg}")
                        result['errors'].append(f"Message {mid}: {str(gmail_msg)}")
                        continue

                    thread_id = gmail_msg.get('threadId')
                    if thread_id:
                        thread_ids_needed.add(thread_id)
                        valid_messages.append((mid, gmail_msg))

                # Step 4: Batch fetch/create threads (single query + inserts)
                thread_map = await self._get_or_create_threads_batch(
                    session, list(thread_ids_needed)
                )

                # Step 5: Create all email messages
                email_messages = []
                for mid, gmail_msg in valid_messages:
                    try:
                        thread_id = gmail_msg['threadId']
                        email_thread = thread_map.get(thread_id)

                        if not email_thread:
                            logger.warning(f"Thread {thread_id} not found for message {mid}")
                            continue

                        # Extract metadata and content
                        extracted_metadata = self.metadata_extractor.extract_metadata(gmail_msg)
                        headers = self._extract_headers(gmail_msg['payload'].get('headers', []))
                        content = self._extract_message_content(gmail_msg['payload'])

                        # Create message record
                        email_message = EmailMessage(
                            gmail_message_id=mid,
                            thread_id=email_thread.id,
                            subject=headers.get('subject'),
                            from_email=headers.get('from_email', ''),
                            from_name=headers.get('from_name'),
                            to_emails=self._parse_email_addresses(headers.get('to', '')),
                            cc_emails=self._parse_email_addresses(headers.get('cc', '')) if headers.get('cc') else None,
                            bcc_emails=self._parse_email_addresses(headers.get('bcc', '')) if headers.get('bcc') else None,
                            body_text=content.get('text'),
                            body_html=content.get('html'),
                            snippet=gmail_msg.get('snippet'),
                            gmail_labels=gmail_msg.get('labelIds', []),
                            internal_date=datetime.fromtimestamp(int(gmail_msg['internalDate']) / 1000),
                            is_unread='UNREAD' in gmail_msg.get('labelIds', []),
                            size_estimate=gmail_msg.get('sizeEstimate'),
                            has_attachments=content['has_attachments'],
                            attachment_count=content['attachment_count'],
                            processing_status='processing'
                        )

                        email_messages.append({
                            'message': email_message,
                            'thread': email_thread,
                            'gmail_msg': gmail_msg,
                            'extracted_metadata': extracted_metadata,
                            'content': content
                        })

                    except Exception as e:
                        logger.error(f"Failed to prepare message {mid}: {e}")
                        result['errors'].append(f"Message {mid}: {str(e)}")

                # Step 6: Batch insert all messages
                if email_messages:
                    session.add_all([em['message'] for em in email_messages])
                    await session.flush()  # Get IDs without committing

                    # Update thread statistics in batch
                    for em_data in email_messages:
                        thread = em_data['thread']
                        msg = em_data['message']

                        thread.message_count = (thread.message_count or 0) + 1
                        if not thread.first_message_date or msg.internal_date < thread.first_message_date:
                            thread.first_message_date = msg.internal_date
                        if not thread.last_message_date or msg.internal_date > thread.last_message_date:
                            thread.last_message_date = msg.internal_date

                        result['thread_ids'].add(thread.gmail_thread_id)

                    await session.commit()

                    # Process attachments and publish events (after commit)
                    for em_data in email_messages:
                        msg = em_data['message']
                        content = em_data['content']

                        # Process attachments
                        attachment_count = 0
                        if content['attachments']:
                            attachment_count = await self._process_attachments(
                                session, msg, content['attachments']
                            )
                        result['attachment_count'] += attachment_count

                        # Mark as processed
                        msg.processing_status = 'completed'
                        msg.event_published_at = datetime.utcnow()

                        # Publish event
                        await self._publish_enhanced_email_event(
                            email_message=msg,
                            email_thread=em_data['thread'],
                            extracted_metadata=em_data['extracted_metadata'],
                            attachment_count=attachment_count
                        )

                        result['processed_count'] += 1

                    await session.commit()

            except Exception as e:
                logger.error(f"Batch processing failed: {e}")
                result['errors'].append(f"Batch error: {str(e)}")
                await session.rollback()

        return result

    async def _get_or_create_threads_batch(
        self,
        session: AsyncSession,
        gmail_thread_ids: List[str]
    ) -> Dict[str, 'EmailThread']:
        """Get or create multiple threads efficiently.

        Args:
            session: Database session
            gmail_thread_ids: List of Gmail thread IDs

        Returns:
            Dict mapping gmail_thread_id to EmailThread
        """
        if not gmail_thread_ids:
            return {}

        # Batch lookup existing threads
        existing_query = select(EmailThread).where(
            and_(
                EmailThread.gmail_thread_id.in_(gmail_thread_ids),
                EmailThread.connection_id == self.connection_id
            )
        )
        existing_result = await session.execute(existing_query)
        thread_map = {
            thread.gmail_thread_id: thread
            for thread in existing_result.scalars().all()
        }

        # Create missing threads
        missing_ids = set(gmail_thread_ids) - set(thread_map.keys())
        if missing_ids:
            new_threads = [
                EmailThread(
                    gmail_thread_id=tid,
                    connection_id=self.connection_id,
                    message_count=0,
                    processing_status='processing'
                )
                for tid in missing_ids
            ]
            session.add_all(new_threads)
            await session.flush()

            for thread in new_threads:
                thread_map[thread.gmail_thread_id] = thread

        return thread_map

    async def _process_single_message(self, message_id: str) -> Optional[Dict[str, Any]]:
        """Process a single Gmail message.

        Note: For batch processing, use _process_message_batch instead.
        This method is retained for backward compatibility and single-message
        operations like incremental sync.

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

            # Extract comprehensive metadata using metadata extractor
            extracted_metadata = self.metadata_extractor.extract_metadata(message_data)

            # Extract message metadata (legacy method for compatibility)
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

            # Publish enhanced email ingested event with rich metadata
            await self._publish_enhanced_email_event(
                email_message=email_message,
                email_thread=email_thread,
                extracted_metadata=extracted_metadata,
                attachment_count=attachment_count
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


    async def setup_push_notifications(
        self,
        topic_name: str,
        label_ids: Optional[List[str]] = None
    ) -> Dict[str, Any]:
        """Set up Gmail push notifications.

        Args:
            topic_name: Cloud Pub/Sub topic name
            label_ids: Labels to watch (default: all labels)

        Returns:
            Watch response with expiration details
        """
        logger.info(f"Setting up push notifications for connection {self.connection_id}")

        try:
            # Setup push notifications
            watch_response = await self.gmail_client.watch_mailbox(
                topic_name=topic_name,
                label_ids=label_ids
            )

            # Update connection with push subscription details
            async with db_manager.get_session() as session:
                result = await session.execute(
                    select(GmailConnection).where(
                        GmailConnection.id == self.connection_id
                    )
                )
                connection = result.scalar_one_or_none()

                if connection:
                    connection.push_topic_name = topic_name
                    # Convert expiration to datetime (Gmail returns milliseconds)
                    if 'expiration' in watch_response:
                        expiration_ms = int(watch_response['expiration'])
                        connection.push_subscription_expiry = datetime.fromtimestamp(
                            expiration_ms / 1000
                        )
                    # Update current history ID
                    connection.current_history_id = watch_response.get('historyId')
                    await session.commit()

            logger.info(f"Push notifications setup complete: {watch_response}")
            return watch_response

        except Exception as e:
            logger.error(f"Failed to setup push notifications: {e}")
            raise

    async def stop_push_notifications(self) -> None:
        """Stop Gmail push notifications."""
        try:
            await self.gmail_client.stop_watch()

            # Clear push subscription details
            async with db_manager.get_session() as session:
                result = await session.execute(
                    select(GmailConnection).where(
                        GmailConnection.id == self.connection_id
                    )
                )
                connection = result.scalar_one_or_none()

                if connection:
                    connection.push_topic_name = None
                    connection.push_subscription_expiry = None
                    await session.commit()

            logger.info(f"Push notifications stopped for connection {self.connection_id}")

        except Exception as e:
            logger.error(f"Failed to stop push notifications: {e}")
            raise

    async def sync_incremental_emails(
        self,
        start_history_id: str,
        end_history_id: Optional[str] = None
    ) -> Dict[str, Any]:
        """Perform incremental email synchronization using History API.

        Args:
            start_history_id: History ID to start from
            end_history_id: History ID to end at (optional)

        Returns:
            Sync summary with statistics
        """
        start_time = datetime.utcnow()

        logger.info(f"Starting incremental sync for connection {self.connection_id}")
        logger.info(f"History range: {start_history_id} -> {end_history_id or 'latest'}")

        await self.sync_publisher.publish_sync_progress(
            sync_session_id=self.sync_session_id,
            connection_id=self.connection_id,
            sync_type="incremental",
            processed_items=0,
            current_batch=0,
            status="starting",
            current_operation="Fetching history changes"
        )

        try:
            messages_processed = 0
            threads_processed = 0
            errors = []
            page_token = None

            # Get all history changes
            while True:
                try:
                    history_data = await self.gmail_client.get_history(
                        start_history_id=start_history_id,
                        max_results=100,
                        page_token=page_token
                    )

                    if 'history' not in history_data:
                        break

                    # Process each history record
                    batch_messages = 0
                    batch_threads = set()

                    for history_record in history_data['history']:
                        # Stop if we've reached the end history ID
                        if end_history_id and history_record['id'] >= end_history_id:
                            break

                        # Process messages added
                        if 'messagesAdded' in history_record:
                            for msg_added in history_record['messagesAdded']:
                                try:
                                    message_id = msg_added['message']['id']
                                    result = await self._process_single_message(message_id)
                                    if result:
                                        batch_messages += 1
                                        batch_threads.add(result['thread_id'])

                                except Exception as e:
                                    logger.error(f"Failed to process added message: {e}")
                                    errors.append(str(e))

                        # Process messages deleted
                        if 'messagesDeleted' in history_record:
                            for msg_deleted in history_record['messagesDeleted']:
                                try:
                                    await self._handle_message_deleted(
                                        msg_deleted['message']['id']
                                    )
                                except Exception as e:
                                    logger.error(f"Failed to handle deleted message: {e}")

                        # Process label changes (optional)
                        if 'labelsAdded' in history_record:
                            await self._handle_labels_added(history_record['labelsAdded'])

                        if 'labelsRemoved' in history_record:
                            await self._handle_labels_removed(history_record['labelsRemoved'])

                    messages_processed += batch_messages
                    threads_processed += len(batch_threads)

                    # Publish progress
                    await self.sync_publisher.publish_sync_progress(
                        sync_session_id=self.sync_session_id,
                        connection_id=self.connection_id,
                        sync_type="incremental",
                        processed_items=messages_processed,
                        current_batch=messages_processed // 50 + 1,
                        status="in_progress",
                        current_operation=f"Processing history changes"
                    )

                    # Check for more pages
                    page_token = history_data.get('nextPageToken')
                    if not page_token:
                        break

                except Exception as e:
                    logger.error(f"Error fetching history: {e}")
                    errors.append(str(e))
                    break

            # Calculate performance metrics
            duration = (datetime.utcnow() - start_time).total_seconds()
            items_per_minute = (messages_processed / duration * 60) if duration > 0 else 0

            # Update connection with latest history ID
            async with db_manager.get_session() as session:
                result = await session.execute(
                    select(GmailConnection).where(
                        GmailConnection.id == self.connection_id
                    )
                )
                connection = result.scalar_one_or_none()

                if connection:
                    connection.current_history_id = end_history_id or start_history_id
                    connection.last_sync = datetime.utcnow()
                    await session.commit()

            # Publish completion event
            await self.sync_publisher.publish_sync_completed(
                sync_session_id=self.sync_session_id,
                connection_id=self.connection_id,
                sync_type="incremental",
                total_messages=messages_processed,
                total_threads=threads_processed,
                duration_seconds=duration,
                success=len(errors) == 0,
                failed_messages=len(errors),
                error_summary="; ".join(errors[:3]) if errors else None
            )

            logger.info(f"Incremental sync completed: {messages_processed} messages, "
                       f"{threads_processed} threads in {duration:.2f}s")

            return {
                'success': len(errors) == 0,
                'messages_processed': messages_processed,
                'threads_processed': threads_processed,
                'duration_seconds': duration,
                'items_per_minute': items_per_minute,
                'errors': errors
            }

        except Exception as e:
            logger.error(f"Incremental sync failed: {e}")
            await self.sync_publisher.publish_sync_completed(
                sync_session_id=self.sync_session_id,
                connection_id=self.connection_id,
                sync_type="incremental",
                total_messages=0,
                total_threads=0,
                duration_seconds=(datetime.utcnow() - start_time).total_seconds(),
                success=False,
                error_summary=str(e)
            )
            raise

    async def _handle_message_deleted(self, gmail_message_id: str) -> None:
        """Handle a message that was deleted from Gmail.

        Args:
            gmail_message_id: Gmail message ID that was deleted
        """
        async with db_manager.get_session() as session:
            # Find the message in our database
            result = await session.execute(
                select(EmailMessage).where(
                    EmailMessage.gmail_message_id == gmail_message_id
                )
            )
            message = result.scalar_one_or_none()

            if message:
                # Mark message as deleted (soft delete)
                message.processing_status = 'deleted'
                await session.commit()
                logger.info(f"Marked message {gmail_message_id} as deleted")
            else:
                logger.debug(f"Deleted message {gmail_message_id} not found in database")

    async def _handle_labels_added(self, labels_added: List[Dict[str, Any]]) -> None:
        """Handle label additions from history.

        Args:
            labels_added: List of label addition events
        """
        for label_event in labels_added:
            try:
                message_id = label_event['message']['id']
                label_ids = label_event.get('labelIds', [])

                # Update message labels in database
                async with db_manager.get_session() as session:
                    result = await session.execute(
                        select(EmailMessage).where(
                            EmailMessage.gmail_message_id == message_id
                        )
                    )
                    message = result.scalar_one_or_none()

                    if message:
                        # Add new labels to existing labels
                        current_labels = set(message.gmail_labels or [])
                        current_labels.update(label_ids)
                        message.gmail_labels = list(current_labels)
                        await session.commit()

            except Exception as e:
                logger.error(f"Error handling added labels: {e}")

    async def _handle_labels_removed(self, labels_removed: List[Dict[str, Any]]) -> None:
        """Handle label removals from history.

        Args:
            labels_removed: List of label removal events
        """
        for label_event in labels_removed:
            try:
                message_id = label_event['message']['id']
                label_ids = label_event.get('labelIds', [])

                # Update message labels in database
                async with db_manager.get_session() as session:
                    result = await session.execute(
                        select(EmailMessage).where(
                            EmailMessage.gmail_message_id == message_id
                        )
                    )
                    message = result.scalar_one_or_none()

                    if message:
                        # Remove labels from existing labels
                        current_labels = set(message.gmail_labels or [])
                        current_labels.difference_update(label_ids)
                        message.gmail_labels = list(current_labels)
                        await session.commit()

            except Exception as e:
                logger.error(f"Error handling removed labels: {e}")

    async def _publish_enhanced_email_event(
        self,
        email_message: EmailMessage,
        email_thread: EmailThread,
        extracted_metadata: ExtractedMetadata,
        attachment_count: int
    ) -> None:
        """Publish enhanced email ingested event with rich metadata.

        Args:
            email_message: Email message record
            email_thread: Email thread record
            extracted_metadata: Extracted metadata object
            attachment_count: Number of attachments
        """
        try:
            # Create entity resolution hints
            entity_resolution_hints = {
                'unique_participants': extracted_metadata.participants.get('unique_participants', []),
                'participant_count': extracted_metadata.participants.get('participant_count', 0),
                'has_external_participants': extracted_metadata.participants.get('has_external_participants', False),
                'domain_organizations': self._extract_domain_organizations(extracted_metadata.participants),
                'suggested_entities': self._generate_entity_suggestions(extracted_metadata.participants)
            }

            # Publish enhanced event with all metadata
            await self.email_publisher.publish_email_ingested(
                # Basic identification
                gmail_message_id=extracted_metadata.gmail_message_id,
                gmail_thread_id=extracted_metadata.gmail_thread_id,
                connection_id=self.connection_id,
                message_id_header=extracted_metadata.message_id_header,

                # Enhanced participant information
                participants=extracted_metadata.participants,
                unique_participant_count=extracted_metadata.participants.get('participant_count', 0),
                has_external_participants=extracted_metadata.participants.get('has_external_participants', False),

                # Enhanced subject and threading
                subject=extracted_metadata.subject,
                normalized_subject=extracted_metadata.normalized_subject,
                subject_hash=extracted_metadata.subject_hash,

                # Legacy participant fields (for backward compatibility)
                from_email=email_message.from_email,
                from_name=email_message.from_name,
                to_emails=email_message.to_emails,
                cc_emails=email_message.cc_emails,
                bcc_emails=email_message.bcc_emails,

                # Thread relationship information
                thread_info=extracted_metadata.thread_info.to_dict(),
                is_root_message=extracted_metadata.thread_info.is_root_message,
                reply_depth=extracted_metadata.thread_info.reply_depth,
                parent_message_id=extracted_metadata.thread_info.parent_message_id,
                references=extracted_metadata.thread_info.references,

                # Enhanced content information
                content_structure=extracted_metadata.content_structure.to_dict(),
                message_format=extracted_metadata.content_structure.message_format,
                has_body_text=extracted_metadata.content_structure.has_plain_text,
                has_body_html=extracted_metadata.content_structure.has_html,
                has_attachments=extracted_metadata.content_structure.has_attachments,
                attachment_count=attachment_count,
                content_types=extracted_metadata.content_structure.content_types,

                # Enhanced priority and importance
                priority_info=extracted_metadata.priority.to_dict(),
                importance_level=extracted_metadata.priority.importance_level,
                priority_score=extracted_metadata.priority.priority_score,
                urgency_indicators=extracted_metadata.priority.urgency_indicators,
                is_automated=extracted_metadata.priority.is_automated,

                # Enhanced Gmail metadata
                internal_date=extracted_metadata.internal_date,
                sent_date=extracted_metadata.sent_date,
                received_date=extracted_metadata.received_date,
                gmail_labels=extracted_metadata.gmail_labels,
                is_unread=extracted_metadata.is_unread,
                is_starred=extracted_metadata.is_starred,
                is_important=extracted_metadata.is_important,
                size_estimate=extracted_metadata.size_estimate,

                # Security and authenticity
                authentication_results=extracted_metadata.authentication_results,
                spam_score=extracted_metadata.spam_score,

                # Thread information
                thread_message_count=email_thread.message_count,
                is_first_message=email_thread.message_count == 1,

                # Processing context
                content_priority=extracted_metadata.priority.importance_level,
                requires_ai_processing=True,
                entity_resolution_hints=entity_resolution_hints,

                # Additional metadata
                custom_headers=extracted_metadata.custom_headers,
                delivery_info=extracted_metadata.delivery_info
            )

            logger.debug(f"Published enhanced email event for message {extracted_metadata.gmail_message_id}")

        except Exception as e:
            logger.error(f"Failed to publish enhanced email event: {e}")
            # Fallback to basic event
            await self._publish_basic_email_event(email_message, email_thread, attachment_count)

    def _extract_domain_organizations(self, participants: Dict[str, Any]) -> Dict[str, str]:
        """Extract organization hints from participant domains.

        Args:
            participants: Participants dictionary

        Returns:
            Domain to organization mapping
        """
        domain_orgs = {}
        unique_participants = participants.get('unique_participants', [])

        for participant in unique_participants:
            email = participant.get('normalized_email') or participant.get('email', '')
            if '@' in email:
                domain = email.split('@')[1].lower()

                # Simple heuristics for common organizations
                if domain.endswith('.edu'):
                    domain_orgs[domain] = 'Educational Institution'
                elif domain.endswith('.gov'):
                    domain_orgs[domain] = 'Government Organization'
                elif domain.endswith('.org'):
                    domain_orgs[domain] = 'Non-Profit Organization'
                elif domain in ['gmail.com', 'yahoo.com', 'hotmail.com', 'outlook.com']:
                    domain_orgs[domain] = 'Personal Email Provider'
                else:
                    # Extract likely organization name from domain
                    base_domain = domain.split('.')[0]
                    domain_orgs[domain] = base_domain.replace('-', ' ').title()

        return domain_orgs

    def _generate_entity_suggestions(self, participants: Dict[str, Any]) -> List[Dict[str, Any]]:
        """Generate entity resolution suggestions.

        Args:
            participants: Participants dictionary

        Returns:
            List of entity suggestions
        """
        suggestions = []
        unique_participants = participants.get('unique_participants', [])

        for participant in unique_participants:
            normalized_email = participant.get('normalized_email')
            normalized_name = participant.get('normalized_name')
            confidence = participant.get('confidence_score', 0.5)

            if normalized_email:
                suggestion = {
                    'entity_type': 'person',
                    'primary_identifier': normalized_email,
                    'display_name': normalized_name,
                    'confidence_score': confidence,
                    'suggested_canonical_name': normalized_name,
                    'suggested_aliases': [participant.get('email'), participant.get('display_name')],
                    'organization_hint': self._infer_organization(normalized_email),
                    'is_internal': self._is_internal_participant(normalized_email)
                }
                suggestions.append(suggestion)

        return suggestions

    def _infer_organization(self, email: str) -> Optional[str]:
        """Infer organization from email domain.

        Args:
            email: Email address

        Returns:
            Inferred organization name
        """
        if '@' not in email:
            return None

        domain = email.split('@')[1].lower()

        # Handle known personal domains
        personal_domains = ['gmail.com', 'yahoo.com', 'hotmail.com', 'outlook.com', 'icloud.com']
        if domain in personal_domains:
            return None

        # Extract organization name from domain
        parts = domain.split('.')
        if len(parts) >= 2:
            return parts[0].replace('-', ' ').title()

        return None

    def _is_internal_participant(self, email: str) -> bool:
        """Determine if participant is internal to the organization.

        Args:
            email: Email address

        Returns:
            True if participant appears to be internal
        """
        # This is a simple heuristic - in practice this would check against
        # known internal domains for the organization
        if '@' not in email:
            return False

        domain = email.split('@')[1].lower()

        # Google employees as an example of "internal" for demo purposes
        return domain in ['google.com', 'gmail.com']

    async def _publish_basic_email_event(
        self,
        email_message: EmailMessage,
        email_thread: EmailThread,
        attachment_count: int
    ) -> None:
        """Fallback to publish basic email event.

        Args:
            email_message: Email message record
            email_thread: Email thread record
            attachment_count: Number of attachments
        """
        await self.email_publisher.publish_email_ingested(
            gmail_message_id=email_message.gmail_message_id,
            gmail_thread_id=email_thread.gmail_thread_id,
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

    # Create Gmail client with connection ID for error tracking
    gmail_client = GmailClient(credentials, connection_id=connection_id)

    # Create event publishers
    email_publisher = EmailEventPublisher()
    sync_publisher = SyncEventPublisher()

    return GmailSyncManager(
        connection_id=connection_id,
        gmail_client=gmail_client,
        email_publisher=email_publisher,
        sync_publisher=sync_publisher
    )