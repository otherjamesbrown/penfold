"""Unit tests for Gmail privacy filters."""

import pytest
from unittest.mock import Mock, AsyncMock, patch
from datetime import datetime, timedelta

from penf_lib.connectors.gmail.privacy import GmailPrivacyFilter, PrivacyFilterResult
from penf_lib.connectors.gmail.metadata import ExtractedMetadata, ThreadRelationship, MessagePriority, ContentStructure
from penf_lib.models.privacy_filter import PrivacyFilter, FilterType, FilterAction


class TestGmailPrivacyFilter:
    """Test Gmail privacy filter functionality."""

    def setup_method(self):
        """Set up test fixtures."""
        self.privacy_filter = GmailPrivacyFilter()

    def create_sample_metadata(self, **overrides):
        """Create sample extracted metadata for testing."""
        metadata = ExtractedMetadata(
            gmail_message_id='test_msg_123',
            gmail_thread_id='test_thread_456',
            message_id_header='<test@example.com>',
            thread_info=ThreadRelationship(
                parent_message_id=None,
                references=[],
                in_reply_to=None,
                position_in_thread=0,
                is_root_message=True,
                reply_depth=0
            ),
            participants={
                'unique_participants': [
                    {'email': 'sender@example.com', 'normalized_email': 'sender@example.com', 'display_name': 'Test Sender'},
                    {'email': 'recipient@company.com', 'normalized_email': 'recipient@company.com', 'display_name': 'Test Recipient'}
                ],
                'participant_count': 2,
                'has_external_participants': True
            },
            content_structure=ContentStructure(
                has_plain_text=True,
                has_html=False,
                has_attachments=False,
                is_multipart=False,
                content_types=['text/plain'],
                message_format='plain',
                encoding='utf-8'
            ),
            priority=MessagePriority(
                importance_level='normal',
                priority_score=0.5,
                urgency_indicators=[],
                is_automated=False,
                confidence_score=0.9
            ),
            gmail_labels=['INBOX', 'UNREAD'],
            is_unread=True,
            is_starred=False,
            is_important=False,
            size_estimate=1024,
            internal_date=datetime.utcnow(),
            sent_date=datetime.utcnow(),
            received_date=datetime.utcnow(),
            spam_score=None,
            virus_scan_result=None,
            authentication_results={},
            subject='Test Email Subject',
            normalized_subject='Test Email Subject',
            subject_hash='test_hash_123',
            custom_headers={},
            delivery_info={}
        )

        # Apply overrides
        for key, value in overrides.items():
            setattr(metadata, key, value)

        return metadata

    def create_sample_gmail_message(self, **overrides):
        """Create sample Gmail message for testing."""
        message = {
            'id': 'test_msg_123',
            'threadId': 'test_thread_456',
            'labelIds': ['INBOX', 'UNREAD'],
            'snippet': 'This is a test email content snippet',
            'sizeEstimate': 1024,
            'payload': {
                'headers': [
                    {'name': 'From', 'value': 'sender@example.com'},
                    {'name': 'To', 'value': 'recipient@company.com'},
                    {'name': 'Subject', 'value': 'Test Email Subject'}
                ]
            }
        }

        message.update(overrides)
        return message

    @pytest.mark.asyncio
    async def test_label_based_filter_match(self):
        """Test label-based privacy filter matching."""
        # Create filter that blocks SPAM emails
        filter_config = {
            'labels': ['SPAM', 'PROMOTIONS'],
            'match_any': True
        }

        gmail_message = self.create_sample_gmail_message(labelIds=['INBOX', 'SPAM'])
        metadata = self.create_sample_metadata()

        result = await self.privacy_filter._evaluate_label_filter(filter_config, gmail_message)

        assert result['matched'] is True
        assert 'SPAM' in result['reason']
        assert 'SPAM' in result['criteria']['matched_labels']

    @pytest.mark.asyncio
    async def test_label_based_filter_no_match(self):
        """Test label-based filter with no match."""
        filter_config = {
            'labels': ['SPAM', 'PROMOTIONS'],
            'match_any': True
        }

        gmail_message = self.create_sample_gmail_message(labelIds=['INBOX', 'UNREAD'])
        metadata = self.create_sample_metadata()

        result = await self.privacy_filter._evaluate_label_filter(filter_config, gmail_message)

        assert result['matched'] is False

    @pytest.mark.asyncio
    async def test_participant_based_filter_email_match(self):
        """Test participant-based filter matching by email."""
        filter_config = {
            'participants': ['blocked@example.com', 'spam@test.com'],
            'match_type': 'email',
            'case_sensitive': False,
            'match_any': True
        }

        metadata = self.create_sample_metadata(
            participants={
                'unique_participants': [
                    {'email': 'blocked@example.com', 'normalized_email': 'blocked@example.com'},
                    {'email': 'other@company.com', 'normalized_email': 'other@company.com'}
                ]
            }
        )

        result = await self.privacy_filter._evaluate_participant_filter(filter_config, metadata)

        assert result['matched'] is True
        assert 'blocked@example.com' in result['criteria']['matched_participants']

    @pytest.mark.asyncio
    async def test_domain_based_filter_match(self):
        """Test domain-based privacy filter matching."""
        filter_config = {
            'domains': ['suspicious-domain.com', 'spam-site.net'],
            'include_subdomains': True,
            'match_any': True
        }

        metadata = self.create_sample_metadata(
            participants={
                'unique_participants': [
                    {'email': 'user@sub.suspicious-domain.com', 'normalized_email': 'user@sub.suspicious-domain.com'}
                ]
            }
        )

        result = await self.privacy_filter._evaluate_domain_filter(filter_config, metadata)

        assert result['matched'] is True
        assert 'sub.suspicious-domain.com' in result['criteria']['matched_domains']

    @pytest.mark.asyncio
    async def test_domain_based_filter_no_subdomain(self):
        """Test domain filter with subdomain matching disabled."""
        filter_config = {
            'domains': ['suspicious-domain.com'],
            'include_subdomains': False,
            'match_any': True
        }

        metadata = self.create_sample_metadata(
            participants={
                'unique_participants': [
                    {'email': 'user@sub.suspicious-domain.com', 'normalized_email': 'user@sub.suspicious-domain.com'}
                ]
            }
        )

        result = await self.privacy_filter._evaluate_domain_filter(filter_config, metadata)

        assert result['matched'] is False

    @pytest.mark.asyncio
    async def test_content_pattern_filter_subject_match(self):
        """Test content pattern filter matching in subject."""
        filter_config = {
            'patterns': [r'\bconfidential\b', r'\bsecret\b'],
            'match_subject': True,
            'match_body': False,
            'case_sensitive': False,
            'match_any_pattern': True
        }

        gmail_message = self.create_sample_gmail_message()
        metadata = self.create_sample_metadata(subject='This is CONFIDENTIAL information')

        result = await self.privacy_filter._evaluate_content_pattern_filter(
            filter_config, gmail_message, metadata
        )

        assert result['matched'] is True
        assert 'subject' in result['criteria']['match_locations']

    @pytest.mark.asyncio
    async def test_content_pattern_filter_body_match(self):
        """Test content pattern filter matching in body snippet."""
        filter_config = {
            'patterns': [r'\bpassword\b', r'\bapi[_\s]*key\b'],
            'match_subject': False,
            'match_body': True,
            'case_sensitive': False,
            'match_any_pattern': True
        }

        gmail_message = self.create_sample_gmail_message(
            snippet='Here is your password: secret123'
        )
        metadata = self.create_sample_metadata()

        result = await self.privacy_filter._evaluate_content_pattern_filter(
            filter_config, gmail_message, metadata
        )

        assert result['matched'] is True
        assert 'snippet' in result['criteria']['match_locations']

    @pytest.mark.asyncio
    async def test_sender_exclusion_filter(self):
        """Test sender exclusion filter."""
        filter_config = {
            'excluded_senders': ['noreply@automated.com', 'system@company.com'],
            'case_sensitive': False
        }

        metadata = self.create_sample_metadata(
            participants={
                'participants_by_role': {
                    'from': [
                        {'normalized_email': 'noreply@automated.com', 'normalized_name': 'No Reply'}
                    ]
                }
            }
        )

        result = await self.privacy_filter._evaluate_sender_exclusion_filter(filter_config, metadata)

        assert result['matched'] is True
        assert 'noreply@automated.com' in result['criteria']['excluded_sender']

    @pytest.mark.asyncio
    async def test_subject_pattern_filter(self):
        """Test subject pattern filter."""
        filter_config = {
            'patterns': [r'\[URGENT\]', r'\bASAP\b'],
            'case_sensitive': False,
            'match_any_pattern': True
        }

        metadata = self.create_sample_metadata(subject='[URGENT] Please respond ASAP')

        result = await self.privacy_filter._evaluate_subject_pattern_filter(filter_config, metadata)

        assert result['matched'] is True
        assert len(result['criteria']['matched_patterns']) >= 1

    @pytest.mark.asyncio
    async def test_sensitive_content_detection(self):
        """Test detection of sensitive content patterns."""
        # Test SSN detection
        gmail_message = self.create_sample_gmail_message(
            snippet='My SSN is 123-45-6789 for verification'
        )
        metadata = self.create_sample_metadata()

        result = await self.privacy_filter._detect_sensitive_content(gmail_message, metadata)
        assert result is True

        # Test credit card detection
        gmail_message = self.create_sample_gmail_message(
            snippet='Credit card: 4532 1234 5678 9012'
        )

        result = await self.privacy_filter._detect_sensitive_content(gmail_message, metadata)
        assert result is True

        # Test no sensitive content
        gmail_message = self.create_sample_gmail_message(
            snippet='This is a normal business email'
        )

        result = await self.privacy_filter._detect_sensitive_content(gmail_message, metadata)
        assert result is False

    def test_action_priority_ordering(self):
        """Test filter action priority ordering."""
        # Higher priority actions should have higher scores
        exclude_priority = self.privacy_filter._get_action_priority(FilterAction.EXCLUDE)
        quarantine_priority = self.privacy_filter._get_action_priority(FilterAction.QUARANTINE)
        log_only_priority = self.privacy_filter._get_action_priority(FilterAction.LOG_ONLY)

        assert exclude_priority > quarantine_priority > log_only_priority

    def test_regex_caching(self):
        """Test regex pattern compilation and caching."""
        pattern = r'\btest\b'

        # First compilation
        regex1 = self.privacy_filter._get_compiled_regex(pattern, case_sensitive=False)
        assert regex1 is not None

        # Should return cached version
        regex2 = self.privacy_filter._get_compiled_regex(pattern, case_sensitive=False)
        assert regex1 is regex2  # Same object from cache

        # Different case sensitivity should create new pattern
        regex3 = self.privacy_filter._get_compiled_regex(pattern, case_sensitive=True)
        assert regex3 is not regex1

        # Invalid regex should return None and cache result
        invalid_regex = self.privacy_filter._get_compiled_regex('[invalid', case_sensitive=False)
        assert invalid_regex is None

        # Second call should return cached None
        invalid_regex2 = self.privacy_filter._get_compiled_regex('[invalid', case_sensitive=False)
        assert invalid_regex2 is None

    @pytest.mark.asyncio
    async def test_attachment_filter_blocked_types(self):
        """Test attachment filter for blocked file types."""
        filter_config = {
            'blocked_types': ['application/x-executable', 'application/zip'],
            'allowed_types': [],
            'max_size_mb': None
        }

        gmail_message = self.create_sample_gmail_message()
        gmail_message['payload']['parts'] = [
            {
                'filename': 'malware.exe',
                'mimeType': 'application/x-executable',
                'body': {'size': 1024}
            }
        ]

        result = await self.privacy_filter._evaluate_attachment_filter(filter_config, gmail_message)

        assert result['matched'] is True
        assert 'application/x-executable' in result['reason']

    @pytest.mark.asyncio
    async def test_combined_filter_and_logic(self):
        """Test combined filter with AND logic."""
        filter_config = {
            'rules': [
                {
                    'type': 'label',
                    'config': {'labels': ['INBOX'], 'match_any': True}
                },
                {
                    'type': 'domain',
                    'config': {'domains': ['example.com'], 'match_any': True}
                }
            ],
            'logic': 'AND'
        }

        # Gmail message with INBOX label
        gmail_message = self.create_sample_gmail_message(labelIds=['INBOX'])

        # Metadata with example.com domain
        metadata = self.create_sample_metadata(
            participants={
                'unique_participants': [
                    {'email': 'user@example.com', 'normalized_email': 'user@example.com'}
                ]
            }
        )

        result = await self.privacy_filter._evaluate_combined_filter(
            filter_config, gmail_message, metadata
        )

        assert result['matched'] is True
        assert len(result['criteria']['matched_rules']) == 2

    @pytest.mark.asyncio
    async def test_combined_filter_or_logic(self):
        """Test combined filter with OR logic."""
        filter_config = {
            'rules': [
                {
                    'type': 'label',
                    'config': {'labels': ['SPAM'], 'match_any': True}
                },
                {
                    'type': 'domain',
                    'config': {'domains': ['suspicious.com'], 'match_any': True}
                }
            ],
            'logic': 'OR'
        }

        # Gmail message with SPAM label (first rule matches)
        gmail_message = self.create_sample_gmail_message(labelIds=['SPAM', 'INBOX'])

        # Metadata with safe domain (second rule doesn't match)
        metadata = self.create_sample_metadata(
            participants={
                'unique_participants': [
                    {'email': 'user@company.com', 'normalized_email': 'user@company.com'}
                ]
            }
        )

        result = await self.privacy_filter._evaluate_combined_filter(
            filter_config, gmail_message, metadata
        )

        assert result['matched'] is True
        assert len(result['criteria']['matched_rules']) == 1

    @pytest.mark.asyncio
    @patch('penf_lib.connectors.gmail.privacy.db_manager')
    async def test_full_email_evaluation_with_filters(self, mock_db_manager):
        """Test full email evaluation with multiple filters."""
        # Mock database session and filters
        mock_session = AsyncMock()
        mock_db_manager.get_session.return_value.__aenter__.return_value = mock_session

        # Create mock filters
        spam_filter = PrivacyFilter(
            id=1,
            name='Spam Filter',
            filter_type=FilterType.LABEL_BASED.value,
            action=FilterAction.EXCLUDE.value,
            priority=10,
            filter_config={'labels': ['SPAM'], 'match_any': True}
        )

        domain_filter = PrivacyFilter(
            id=2,
            name='Domain Filter',
            filter_type=FilterType.DOMAIN_BASED.value,
            action=FilterAction.QUARANTINE.value,
            priority=20,
            filter_config={'domains': ['suspicious.com'], 'match_any': True}
        )

        # Mock filter query result
        mock_result = Mock()
        mock_result.scalars.return_value.all.return_value = [spam_filter, domain_filter]
        mock_session.execute.return_value = mock_result

        # Create test data
        gmail_message = self.create_sample_gmail_message(labelIds=['INBOX', 'SPAM'])
        metadata = self.create_sample_metadata()

        # Mock filter stats update
        mock_session.commit = AsyncMock()

        # Evaluate email
        result = await self.privacy_filter.evaluate_email(1, gmail_message, metadata)

        assert isinstance(result, PrivacyFilterResult)
        assert result.should_exclude is True  # EXCLUDE has higher priority than QUARANTINE
        assert result.action == FilterAction.EXCLUDE
        assert len(result.matched_filters) >= 1

    @pytest.mark.asyncio
    @patch('penf_lib.connectors.gmail.privacy.db_manager')
    async def test_no_filters_configured(self, mock_db_manager):
        """Test evaluation when no filters are configured."""
        # Mock empty filter list
        mock_session = AsyncMock()
        mock_db_manager.get_session.return_value.__aenter__.return_value = mock_session

        mock_result = Mock()
        mock_result.scalars.return_value.all.return_value = []
        mock_session.execute.return_value = mock_result

        gmail_message = self.create_sample_gmail_message()
        metadata = self.create_sample_metadata()

        result = await self.privacy_filter.evaluate_email(1, gmail_message, metadata)

        assert result.should_exclude is False
        assert len(result.matched_filters) == 0
        assert len(result.processing_notes) == 0

    @pytest.mark.asyncio
    async def test_error_handling_in_filter_evaluation(self):
        """Test error handling during filter evaluation."""
        # Create filter with invalid config that will cause error
        invalid_filter = PrivacyFilter(
            id=1,
            name='Invalid Filter',
            filter_type='invalid_type',  # Invalid type
            action=FilterAction.EXCLUDE.value,
            priority=10,
            filter_config={}
        )

        gmail_message = self.create_sample_gmail_message()
        metadata = self.create_sample_metadata()

        # This should not raise an exception but return non-match
        result = await self.privacy_filter._evaluate_single_filter(
            invalid_filter, gmail_message, metadata
        )

        assert result['matched'] is False
        assert 'Unknown filter type' in result['reason']