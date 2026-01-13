"""Unit tests for Gmail metadata extraction."""

import pytest
from datetime import datetime
from unittest.mock import Mock, patch
import json

from penf_lib.connectors.gmail.metadata import GmailMetadataExtractor, ExtractedMetadata
from penf_lib.connectors.gmail.participants import ParticipantNormalizer, EmailParticipant


class TestParticipantNormalizer:
    """Test participant normalization functionality."""

    def setup_method(self):
        """Set up test fixtures."""
        self.normalizer = ParticipantNormalizer()

    def test_normalize_simple_email(self):
        """Test normalizing a simple email address."""
        participants = self.normalizer.normalize_email_header("john.doe@example.com")

        assert len(participants) == 1
        participant = participants[0]
        assert participant.email == "john.doe@example.com"
        assert participant.normalized_email == "john.doe@example.com"
        assert participant.display_name is None

    def test_normalize_email_with_name(self):
        """Test normalizing email with display name."""
        participants = self.normalizer.normalize_email_header("John Doe <john.doe@example.com>")

        assert len(participants) == 1
        participant = participants[0]
        assert participant.email == "john.doe@example.com"
        assert participant.display_name == "John Doe"
        assert participant.normalized_name == "John Doe"
        assert participant.normalized_email == "john.doe@example.com"

    def test_normalize_quoted_name(self):
        """Test normalizing quoted display name."""
        participants = self.normalizer.normalize_email_header('"Doe, John" <john.doe@example.com>')

        assert len(participants) == 1
        participant = participants[0]
        assert participant.normalized_name == "John Doe"  # Should convert "Doe, John" to "John Doe"

    def test_normalize_gmail_address(self):
        """Test Gmail-specific normalization."""
        participants = self.normalizer.normalize_email_header("john.doe+tag@gmail.com")

        assert len(participants) == 1
        participant = participants[0]
        assert participant.email == "john.doe+tag@gmail.com"
        assert participant.normalized_email == "johndoe@gmail.com"  # Removes dots and plus addressing

    def test_normalize_multiple_addresses(self):
        """Test normalizing multiple email addresses."""
        participants = self.normalizer.normalize_email_header(
            "John Doe <john@example.com>, Jane Smith <jane@example.com>"
        )

        assert len(participants) == 2
        assert participants[0].email == "john@example.com"
        assert participants[1].email == "jane@example.com"

    def test_normalize_encoded_header(self):
        """Test normalizing RFC 2047 encoded headers."""
        # This would be an encoded subject like "=?utf-8?B?Sm9obiBEb2U=?= <john@example.com>"
        participants = self.normalizer.normalize_email_header("=?utf-8?B?Sm9obiBEb2U=?= <john@example.com>")

        assert len(participants) == 1
        participant = participants[0]
        assert participant.email == "john@example.com"
        # The encoded part should decode to "John Doe"

    def test_participant_confidence_scoring(self):
        """Test confidence scoring for participants."""
        participants = self.normalizer.normalize_email_header("John Doe <john@example.com>")

        assert len(participants) == 1
        participant = participants[0]
        assert 0.9 <= participant.confidence_score <= 1.0  # High confidence for clean data

    def test_extract_unique_participants(self):
        """Test extracting unique participants across roles."""
        participants = {
            'from': [EmailParticipant(email='john@example.com', normalized_email='john@example.com')],
            'to': [EmailParticipant(email='john@example.com', normalized_email='john@example.com')],
            'cc': [EmailParticipant(email='jane@example.com', normalized_email='jane@example.com')]
        }

        unique = self.normalizer.extract_unique_participants(participants)
        assert len(unique) == 2  # Should deduplicate john@example.com


class TestGmailMetadataExtractor:
    """Test Gmail metadata extraction functionality."""

    def setup_method(self):
        """Set up test fixtures."""
        self.extractor = GmailMetadataExtractor()

    def create_sample_gmail_message(self, **overrides):
        """Create a sample Gmail message for testing."""
        sample = {
            'id': 'msg_123456',
            'threadId': 'thread_789012',
            'labelIds': ['INBOX', 'UNREAD'],
            'sizeEstimate': 2048,
            'internalDate': '1705140000000',  # Unix timestamp in milliseconds
            'payload': {
                'mimeType': 'multipart/alternative',
                'headers': [
                    {'name': 'From', 'value': 'John Doe <john@example.com>'},
                    {'name': 'To', 'value': 'jane@example.com'},
                    {'name': 'Subject', 'value': 'Test Email Subject'},
                    {'name': 'Date', 'value': 'Sat, 13 Jan 2024 10:00:00 -0800'},
                    {'name': 'Message-ID', 'value': '<message123@example.com>'},
                    {'name': 'X-Priority', 'value': '1'},
                ],
                'parts': [
                    {
                        'mimeType': 'text/plain',
                        'body': {'data': 'SGVsbG8gV29ybGQ='}  # Base64 encoded "Hello World"
                    },
                    {
                        'mimeType': 'text/html',
                        'body': {'data': 'PGI+SGVsbG8gV29ybGQ8L2I+'}  # Base64 encoded "<b>Hello World</b>"
                    }
                ]
            }
        }

        # Apply any overrides
        if overrides:
            sample.update(overrides)

        return sample

    def test_extract_basic_metadata(self):
        """Test extracting basic metadata from Gmail message."""
        message = self.create_sample_gmail_message()
        metadata = self.extractor.extract_metadata(message)

        assert isinstance(metadata, ExtractedMetadata)
        assert metadata.gmail_message_id == 'msg_123456'
        assert metadata.gmail_thread_id == 'thread_789012'
        assert metadata.subject == 'Test Email Subject'

    def test_extract_participant_info(self):
        """Test extracting participant information."""
        message = self.create_sample_gmail_message()
        metadata = self.extractor.extract_metadata(message)

        participants = metadata.participants
        assert 'participants_by_role' in participants
        assert 'unique_participants' in participants
        assert 'participant_count' in participants

        # Should have participants from 'from' and 'to' roles
        by_role = participants['participants_by_role']
        assert 'from' in by_role
        assert 'to' in by_role

    def test_extract_thread_relationships(self):
        """Test extracting thread relationship information."""
        headers = [
            {'name': 'Message-ID', 'value': '<current@example.com>'},
            {'name': 'In-Reply-To', 'value': '<parent@example.com>'},
            {'name': 'References', 'value': '<root@example.com> <parent@example.com>'}
        ]
        message = self.create_sample_gmail_message()
        message['payload']['headers'].extend(headers)

        metadata = self.extractor.extract_metadata(message)
        thread_info = metadata.thread_info

        assert thread_info.parent_message_id == 'parent@example.com'
        assert thread_info.in_reply_to == 'parent@example.com'
        assert 'root@example.com' in thread_info.references
        assert 'parent@example.com' in thread_info.references
        assert not thread_info.is_root_message
        assert thread_info.reply_depth == 2

    def test_extract_content_structure(self):
        """Test extracting content structure information."""
        message = self.create_sample_gmail_message()
        metadata = self.extractor.extract_metadata(message)

        content = metadata.content_structure
        assert content.has_plain_text
        assert content.has_html
        assert content.is_multipart
        assert content.message_format == 'multipart'
        assert 'text/plain' in content.content_types
        assert 'text/html' in content.content_types

    def test_extract_priority_information(self):
        """Test extracting priority and importance."""
        message = self.create_sample_gmail_message()
        metadata = self.extractor.extract_metadata(message)

        priority = metadata.priority
        assert priority.importance_level == 'high'  # Due to X-Priority: 1
        assert priority.priority_score >= 0.7
        assert 'high_priority_header' in priority.urgency_indicators

    def test_extract_urgent_subject_indicators(self):
        """Test detecting urgency from subject line."""
        message = self.create_sample_gmail_message()
        # Update subject to include urgent keywords
        for header in message['payload']['headers']:
            if header['name'] == 'Subject':
                header['value'] = 'URGENT: Please respond ASAP'
                break

        metadata = self.extractor.extract_metadata(message)
        priority = metadata.priority

        assert priority.importance_level == 'high'
        assert 'subject_urgent_words' in priority.urgency_indicators

    def test_extract_automated_message_detection(self):
        """Test detecting automated messages."""
        message = self.create_sample_gmail_message()
        # Update from address to automated sender
        for header in message['payload']['headers']:
            if header['name'] == 'From':
                header['value'] = 'noreply@automated-system.com'
                break

        metadata = self.extractor.extract_metadata(message)
        priority = metadata.priority

        assert priority.is_automated
        assert priority.priority_score <= 0.3  # Lower priority for automated

    def test_extract_gmail_labels(self):
        """Test extracting Gmail labels and status."""
        message = self.create_sample_gmail_message()
        message['labelIds'] = ['INBOX', 'UNREAD', 'STARRED', 'IMPORTANT']

        metadata = self.extractor.extract_metadata(message)

        assert 'INBOX' in metadata.gmail_labels
        assert metadata.is_unread
        assert metadata.is_starred
        assert metadata.is_important

    def test_extract_date_information(self):
        """Test extracting temporal information."""
        message = self.create_sample_gmail_message()
        metadata = self.extractor.extract_metadata(message)

        assert isinstance(metadata.internal_date, datetime)
        assert isinstance(metadata.sent_date, datetime)
        assert metadata.received_date == metadata.internal_date

    def test_subject_normalization(self):
        """Test subject line normalization."""
        message = self.create_sample_gmail_message()
        for header in message['payload']['headers']:
            if header['name'] == 'Subject':
                header['value'] = 'Re: Fwd: Re: Original Subject [JIRA]'
                break

        metadata = self.extractor.extract_metadata(message)

        assert metadata.subject == 'Re: Fwd: Re: Original Subject [JIRA]'
        assert metadata.normalized_subject == 'Original Subject'  # Should strip Re:/Fwd: and tags

    def test_authentication_results_extraction(self):
        """Test extracting email authentication results."""
        message = self.create_sample_gmail_message()
        auth_headers = [
            {'name': 'Received-SPF', 'value': 'pass (google.com: domain of john@example.com designates 1.2.3.4 as permitted sender)'},
            {'name': 'DKIM-Signature', 'value': 'v=1; a=rsa-sha256; c=relaxed/relaxed; d=example.com; s=selector'},
            {'name': 'Authentication-Results', 'value': 'gmail.com; dmarc=pass (p=none dis=none) header.from=example.com'}
        ]
        message['payload']['headers'].extend(auth_headers)

        metadata = self.extractor.extract_metadata(message)
        auth_results = metadata.authentication_results

        assert auth_results['spf'] == 'pass'
        assert auth_results['has_dkim']
        assert auth_results['dmarc'] == 'pass'

    def test_spam_score_calculation(self):
        """Test spam score calculation."""
        message = self.create_sample_gmail_message()
        message['labelIds'] = ['SPAM']

        metadata = self.extractor.extract_metadata(message)

        assert metadata.spam_score == 0.9  # High spam score for SPAM label

    def test_custom_headers_extraction(self):
        """Test extracting custom headers."""
        message = self.create_sample_gmail_message()
        custom_headers = [
            {'name': 'X-Custom-Header', 'value': 'custom-value'},
            {'name': 'List-ID', 'value': 'announcements.example.com'},
            {'name': 'X-Mailer', 'value': 'MailSystem 1.0'}
        ]
        message['payload']['headers'].extend(custom_headers)

        metadata = self.extractor.extract_metadata(message)
        custom = metadata.custom_headers

        assert 'x-custom-header' in custom
        assert custom['x-custom-header'] == 'custom-value'

    def test_delivery_info_extraction(self):
        """Test extracting delivery information."""
        message = self.create_sample_gmail_message()
        delivery_headers = [
            {'name': 'List-ID', 'value': 'newsletter@company.com'},
            {'name': 'List-Unsubscribe', 'value': '<mailto:unsubscribe@company.com>'},
            {'name': 'Auto-Submitted', 'value': 'auto-generated'},
            {'name': 'Precedence', 'value': 'bulk'}
        ]
        message['payload']['headers'].extend(delivery_headers)

        metadata = self.extractor.extract_metadata(message)
        delivery = metadata.delivery_info

        assert delivery['list_id'] == 'newsletter@company.com'
        assert delivery['is_mailing_list']
        assert 'unsubscribe_info' in delivery
        assert delivery['auto_submitted'] == 'auto-generated'
        assert delivery['precedence'] == 'bulk'


class TestIntegrationWithSyncManager:
    """Test integration of metadata extraction with sync manager."""

    @pytest.mark.asyncio
    async def test_enhanced_event_publishing(self):
        """Test that enhanced events are published with rich metadata."""
        from penf_lib.connectors.gmail.sync import GmailSyncManager
        from penf_lib.connectors.gmail.metadata import ExtractedMetadata
        from penf_lib.models.email_message import EmailMessage, EmailThread

        # Create mock objects
        mock_client = Mock()
        mock_email_publisher = Mock()
        mock_sync_publisher = Mock()

        # Create sync manager
        sync_manager = GmailSyncManager(
            connection_id=1,
            gmail_client=mock_client,
            email_publisher=mock_email_publisher,
            sync_publisher=mock_sync_publisher
        )

        # Create test data
        email_message = EmailMessage(
            gmail_message_id='test_msg',
            subject='Test Subject',
            from_email='test@example.com',
            from_name='Test User',
            to_emails=['recipient@example.com'],
            internal_date=datetime.now()
        )

        email_thread = EmailThread(
            gmail_thread_id='test_thread',
            subject='Test Thread',
            message_count=1
        )

        # Create mock extracted metadata
        extractor = GmailMetadataExtractor()
        sample_message = {
            'id': 'test_msg',
            'threadId': 'test_thread',
            'labelIds': ['INBOX'],
            'sizeEstimate': 1024,
            'internalDate': '1705140000000',
            'payload': {
                'mimeType': 'text/plain',
                'headers': [
                    {'name': 'From', 'value': 'Test User <test@example.com>'},
                    {'name': 'To', 'value': 'recipient@example.com'},
                    {'name': 'Subject', 'value': 'Test Subject'},
                ]
            }
        }

        extracted_metadata = extractor.extract_metadata(sample_message)

        # Mock the publish method
        mock_email_publisher.publish_email_ingested = Mock()

        # Test the enhanced event publishing
        await sync_manager._publish_enhanced_email_event(
            email_message=email_message,
            email_thread=email_thread,
            extracted_metadata=extracted_metadata,
            attachment_count=0
        )

        # Verify that the enhanced event was published
        mock_email_publisher.publish_email_ingested.assert_called_once()
        call_args = mock_email_publisher.publish_email_ingested.call_args[1]

        # Check that enhanced fields are present
        assert 'participants' in call_args
        assert 'thread_info' in call_args
        assert 'content_structure' in call_args
        assert 'priority_info' in call_args
        assert 'entity_resolution_hints' in call_args