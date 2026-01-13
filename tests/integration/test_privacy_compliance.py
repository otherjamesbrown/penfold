"""Integration tests for privacy compliance and data retention."""

import pytest
from unittest.mock import Mock, AsyncMock, patch
from datetime import datetime, timedelta
import asyncio

from penf_lib.connectors.gmail.privacy import GmailPrivacyFilter
from penf_lib.connectors.gmail.retention import RetentionPolicyEnforcer
from penf_lib.models.privacy_filter import (
    PrivacyFilter, PrivacyAuditLog, RetentionPolicy,
    FilterType, FilterAction
)
from penf_lib.models.email_message import EmailMessage, EmailThread, EmailAttachment


class TestPrivacyCompliance:
    """Test privacy compliance and audit functionality."""

    def setup_method(self):
        """Set up test fixtures."""
        self.privacy_filter = GmailPrivacyFilter()
        self.retention_enforcer = RetentionPolicyEnforcer()

    @pytest.fixture
    def sample_privacy_filters(self):
        """Create sample privacy filters for testing."""
        return [
            PrivacyFilter.create_label_filter(
                connection_id=1,
                name="Block Spam",
                labels=["SPAM", "PROMOTIONS"],
                action=FilterAction.EXCLUDE
            ),
            PrivacyFilter.create_domain_filter(
                connection_id=1,
                name="Block Suspicious Domains",
                domains=["malicious.com", "phishing.net"],
                action=FilterAction.QUARANTINE
            ),
            PrivacyFilter.create_content_pattern_filter(
                connection_id=1,
                name="Detect Sensitive Content",
                patterns=[r'\b\d{3}-\d{2}-\d{4}\b', r'\bpassword\b'],
                action=FilterAction.MARK_SENSITIVE
            )
        ]

    @pytest.mark.asyncio
    async def test_privacy_filter_audit_trail(self, db_session, sample_privacy_filters):
        """Test that privacy filter actions are properly audited."""
        # Add filters to database
        for filter_obj in sample_privacy_filters:
            db_session.add(filter_obj)
        await db_session.commit()

        # Create test email with SPAM label
        gmail_message = {
            'id': 'test_spam_001',
            'threadId': 'thread_001',
            'labelIds': ['SPAM'],
            'snippet': 'This is spam content'
        }

        # Mock metadata
        from penf_lib.connectors.gmail.metadata import ExtractedMetadata
        metadata = Mock(spec=ExtractedMetadata)
        metadata.gmail_message_id = 'test_spam_001'
        metadata.gmail_thread_id = 'thread_001'
        metadata.participants = {'unique_participants': [], 'participant_count': 0}
        metadata.subject = 'Spam Email'
        metadata.subject_hash = 'spam_hash'
        metadata.content_structure = Mock()
        metadata.content_structure.has_attachments = False

        # Mock database operations for privacy filter
        with patch('penf_lib.connectors.gmail.privacy.db_manager') as mock_db_manager:
            mock_session = AsyncMock()
            mock_db_manager.get_session.return_value.__aenter__.return_value = mock_session

            # Mock filter query
            mock_result = Mock()
            mock_result.scalars.return_value.all.return_value = [sample_privacy_filters[0]]
            mock_session.execute.return_value = mock_result

            # Mock audit log creation
            audit_logs = []
            def add_audit_log(log):
                audit_logs.append(log)

            mock_session.add.side_effect = add_audit_log
            mock_session.commit = AsyncMock()

            # Evaluate email
            result = await self.privacy_filter.evaluate_email(1, gmail_message, metadata)

            # Should have matched spam filter
            assert result.should_exclude is True
            assert len(result.matched_filters) == 1

            # Check audit log was created
            assert len(audit_logs) == 1
            audit_log = audit_logs[0]
            assert isinstance(audit_log, PrivacyAuditLog)
            assert audit_log.gmail_message_id == 'test_spam_001'
            assert audit_log.action_taken == FilterAction.EXCLUDE.value
            assert 'SPAM' in audit_log.match_reason

    @pytest.mark.asyncio
    async def test_multiple_filter_priority_handling(self, sample_privacy_filters):
        """Test that multiple matching filters respect priority order."""
        # Create email that matches multiple filters
        gmail_message = {
            'id': 'test_multi_001',
            'threadId': 'thread_001',
            'labelIds': ['SPAM'],  # Matches label filter
            'snippet': 'Email from malicious.com with password: secret'  # Matches content filter
        }

        # Mock metadata with malicious domain
        metadata = Mock()
        metadata.gmail_message_id = 'test_multi_001'
        metadata.gmail_thread_id = 'thread_001'
        metadata.participants = {
            'unique_participants': [
                {'email': 'user@malicious.com', 'normalized_email': 'user@malicious.com'}
            ],
            'participant_count': 1
        }
        metadata.subject = 'Malicious Email'

        # Mock database for all three filters
        with patch('penf_lib.connectors.gmail.privacy.db_manager') as mock_db_manager:
            mock_session = AsyncMock()
            mock_db_manager.get_session.return_value.__aenter__.return_value = mock_session

            mock_result = Mock()
            mock_result.scalars.return_value.all.return_value = sample_privacy_filters
            mock_session.execute.return_value = mock_result
            mock_session.add = Mock()
            mock_session.commit = AsyncMock()

            # Evaluate email
            result = await self.privacy_filter.evaluate_email(1, gmail_message, metadata)

            # Should take highest priority action (EXCLUDE from label filter)
            assert result.action == FilterAction.EXCLUDE
            assert result.should_exclude is True

            # Should have multiple matched filters but highest priority action
            assert len(result.matched_filters) >= 2

    @pytest.mark.asyncio
    async def test_sensitive_content_detection_compliance(self):
        """Test detection of various sensitive content patterns."""
        sensitive_test_cases = [
            ('SSN in subject', 'My SSN: 123-45-6789', True),
            ('Credit card in content', 'Card: 4532 1234 5678 9012', True),
            ('Phone number', 'Call me at (555) 123-4567', True),
            ('API key', 'api_key: sk_test_1234567890abcdef', True),
            ('Password mention', 'Your password is: mysecret123', True),
            ('Bank account', 'Account number: 123456789012', True),
            ('Normal content', 'This is a regular business email', False),
        ]

        for test_name, content, should_detect in sensitive_test_cases:
            gmail_message = {
                'id': 'test_sensitive',
                'threadId': 'thread_001',
                'snippet': content
            }

            metadata = Mock()
            metadata.subject = content

            detected = await self.privacy_filter._detect_sensitive_content(gmail_message, metadata)

            assert detected == should_detect, f"Failed for test case: {test_name}"

    @pytest.mark.asyncio
    async def test_data_retention_policy_enforcement(self, db_session):
        """Test data retention policy enforcement."""
        # Create retention policy
        policy = RetentionPolicy(
            connection_id=1,
            name="Test Retention Policy",
            email_retention_days=30,
            attachment_retention_days=14,
            audit_log_retention_days=90,
            auto_delete_enabled=True,
            is_active=True
        )
        db_session.add(policy)

        # Create old emails that should be deleted
        old_date = datetime.utcnow() - timedelta(days=35)
        recent_date = datetime.utcnow() - timedelta(days=10)

        # Create email thread
        thread = EmailThread(
            connection_id=1,
            gmail_thread_id='old_thread_001',
            subject='Old Thread',
            message_count=2
        )
        db_session.add(thread)
        await db_session.flush()

        # Create old email (should be deleted)
        old_email = EmailMessage(
            connection_id=1,
            gmail_message_id='old_msg_001',
            thread_id=thread.id,
            subject='Old Email',
            from_email='sender@example.com',
            to_emails=['recipient@company.com'],
            internal_date=old_date,
            processing_status='completed'
        )

        # Create recent email (should be kept)
        recent_email = EmailMessage(
            connection_id=1,
            gmail_message_id='recent_msg_001',
            thread_id=thread.id,
            subject='Recent Email',
            from_email='sender@example.com',
            to_emails=['recipient@company.com'],
            internal_date=recent_date,
            processing_status='completed'
        )

        db_session.add(old_email)
        db_session.add(recent_email)
        await db_session.commit()

        # Run retention enforcement (dry run)
        results = await self.retention_enforcer.enforce_retention_policies(
            connection_id=1,
            dry_run=True
        )

        # Should identify old email for deletion
        assert results['connections_processed'] == 1
        assert results['emails_deleted'] >= 1  # Should find at least the old email

        # Verify dry run didn't actually delete anything
        remaining_emails = await db_session.execute(
            select(EmailMessage).where(EmailMessage.connection_id == 1)
        )
        remaining_count = len(remaining_emails.scalars().all())
        assert remaining_count == 2  # Both emails still there in dry run

    @pytest.mark.asyncio
    async def test_audit_log_retention(self, db_session):
        """Test audit log retention enforcement."""
        # Create retention policy with short audit log retention
        policy = RetentionPolicy(
            connection_id=1,
            name="Test Policy",
            audit_log_retention_days=7,
            is_active=True
        )
        db_session.add(policy)

        # Create old and recent audit logs
        old_date = datetime.utcnow() - timedelta(days=10)
        recent_date = datetime.utcnow() - timedelta(days=3)

        old_audit = PrivacyAuditLog(
            gmail_message_id='msg_001',
            gmail_thread_id='thread_001',
            connection_id=1,
            filter_id=1,
            filter_name='Test Filter',
            filter_type='label',
            action_taken='exclude',
            match_reason='Test reason',
            timestamp=old_date
        )

        recent_audit = PrivacyAuditLog(
            gmail_message_id='msg_002',
            gmail_thread_id='thread_002',
            connection_id=1,
            filter_id=1,
            filter_name='Test Filter',
            filter_type='label',
            action_taken='exclude',
            match_reason='Test reason',
            timestamp=recent_date
        )

        db_session.add(old_audit)
        db_session.add(recent_audit)
        await db_session.commit()

        # Test audit log cleanup (dry run)
        results = await self.retention_enforcer._delete_expired_audit_logs(
            db_session, 1, policy
        )

        # Should identify old audit log for deletion
        assert results['count'] >= 1

    @pytest.mark.asyncio
    async def test_specific_email_deletion_compliance(self, db_session):
        """Test deletion of specific email with full audit trail."""
        # Create email with attachments
        thread = EmailThread(
            connection_id=1,
            gmail_thread_id='thread_001',
            subject='Test Thread',
            message_count=1
        )
        db_session.add(thread)
        await db_session.flush()

        email = EmailMessage(
            connection_id=1,
            gmail_message_id='delete_test_001',
            thread_id=thread.id,
            subject='Email to Delete',
            from_email='sender@example.com',
            to_emails=['recipient@company.com'],
            internal_date=datetime.utcnow(),
            processing_status='completed'
        )
        db_session.add(email)
        await db_session.flush()

        # Create attachment
        attachment = EmailAttachment(
            message_id=email.id,
            gmail_attachment_id='attach_001',
            filename='test.pdf',
            mime_type='application/pdf',
            size_bytes=1024,
            file_path='/tmp/test.pdf'
        )
        db_session.add(attachment)

        # Create audit log for the email
        audit_log = PrivacyAuditLog(
            gmail_message_id='delete_test_001',
            gmail_thread_id='thread_001',
            connection_id=1,
            filter_id=1,
            filter_name='Test Filter',
            filter_type='test',
            action_taken='log',
            match_reason='Test'
        )
        db_session.add(audit_log)
        await db_session.commit()

        # Delete specific email
        results = await self.retention_enforcer.delete_specific_email(
            connection_id=1,
            gmail_message_id='delete_test_001',
            delete_reason='User requested deletion'
        )

        # Verify deletion results
        assert results['success'] is True
        assert results['deleted_items']['email_message'] == 1
        assert results['deleted_items']['attachments'] == 1
        assert results['deleted_items']['audit_logs'] == 1

        # Verify email is actually deleted
        remaining_emails = await db_session.execute(
            select(EmailMessage).where(
                EmailMessage.gmail_message_id == 'delete_test_001'
            )
        )
        assert remaining_emails.scalar_one_or_none() is None

        # Verify deletion audit log was created
        deletion_audit = await db_session.execute(
            select(PrivacyAuditLog).where(
                and_(
                    PrivacyAuditLog.gmail_message_id == 'delete_test_001',
                    PrivacyAuditLog.filter_name == 'manual_deletion'
                )
            )
        )
        deletion_log = deletion_audit.scalar_one_or_none()
        assert deletion_log is not None
        assert deletion_log.action_taken == 'delete'

    @pytest.mark.asyncio
    async def test_privacy_filter_performance_monitoring(self, db_session):
        """Test privacy filter performance tracking."""
        # Create filter
        filter_obj = PrivacyFilter.create_label_filter(
            connection_id=1,
            name="Performance Test Filter",
            labels=["TEST"]
        )
        db_session.add(filter_obj)
        await db_session.commit()

        # Simulate filter matches with timing
        initial_match_count = filter_obj.match_count

        # Update filter stats multiple times
        for i in range(5):
            processing_time = 50 + i * 10  # Varying processing times
            await self.privacy_filter._update_filter_stats(filter_obj, processing_time)

        # Refresh filter to get updated stats
        await db_session.refresh(filter_obj)

        # Verify statistics are tracked
        assert filter_obj.match_count == initial_match_count + 5
        assert filter_obj.avg_processing_time_ms > 0
        assert filter_obj.last_matched is not None

        # Test statistics retrieval
        stats = await self.privacy_filter.get_filter_statistics(connection_id=1, days=1)

        assert 'filter_performance' in stats
        assert 'action_counts' in stats
        assert stats['period_days'] == 1

    @pytest.mark.asyncio
    async def test_gdpr_compliance_data_export(self, db_session):
        """Test GDPR compliance data export functionality."""
        # Create test data
        email = EmailMessage(
            connection_id=1,
            gmail_message_id='gdpr_test_001',
            subject='GDPR Test Email',
            from_email='user@example.com',
            to_emails=['recipient@company.com'],
            internal_date=datetime.utcnow(),
            processing_status='completed'
        )
        db_session.add(email)

        audit_log = PrivacyAuditLog(
            gmail_message_id='gdpr_test_001',
            gmail_thread_id='thread_001',
            connection_id=1,
            filter_id=1,
            filter_name='GDPR Filter',
            filter_type='participant',
            action_taken='log',
            match_reason='GDPR compliance test'
        )
        db_session.add(audit_log)
        await db_session.commit()

        # Test data export for specific user
        user_data_query = select(EmailMessage, PrivacyAuditLog).join(
            PrivacyAuditLog,
            EmailMessage.gmail_message_id == PrivacyAuditLog.gmail_message_id
        ).where(
            or_(
                EmailMessage.from_email == 'user@example.com',
                EmailMessage.to_emails.contains(['user@example.com'])
            )
        )

        result = await db_session.execute(user_data_query)
        user_data = result.all()

        # Should find the test email and audit log
        assert len(user_data) >= 1

        # Format for export (simplified)
        exported_data = {
            'emails': [
                {
                    'message_id': email.gmail_message_id,
                    'subject': email.subject,
                    'date': email.internal_date.isoformat(),
                    'from': email.from_email,
                    'to': email.to_emails
                }
                for email, audit in user_data
            ],
            'privacy_actions': [
                {
                    'message_id': audit.gmail_message_id,
                    'action': audit.action_taken,
                    'reason': audit.match_reason,
                    'timestamp': audit.timestamp.isoformat()
                }
                for email, audit in user_data
            ]
        }

        assert len(exported_data['emails']) >= 1
        assert len(exported_data['privacy_actions']) >= 1

    @pytest.mark.asyncio
    async def test_retention_status_reporting(self, db_session):
        """Test retention policy status reporting."""
        # Create retention policy
        policy = RetentionPolicy(
            connection_id=1,
            name="Status Test Policy",
            email_retention_days=365,
            attachment_retention_days=90,
            is_active=True,
            last_cleanup_run=datetime.utcnow() - timedelta(days=1),
            next_cleanup_scheduled=datetime.utcnow() + timedelta(days=1)
        )
        db_session.add(policy)

        # Create test emails with different ages
        recent_email = EmailMessage(
            connection_id=1,
            gmail_message_id='recent_001',
            subject='Recent Email',
            from_email='sender@example.com',
            to_emails=['recipient@company.com'],
            internal_date=datetime.utcnow() - timedelta(days=10)
        )

        old_email = EmailMessage(
            connection_id=1,
            gmail_message_id='old_001',
            subject='Old Email',
            from_email='sender@example.com',
            to_emails=['recipient@company.com'],
            internal_date=datetime.utcnow() - timedelta(days=400)  # Past retention
        )

        db_session.add(recent_email)
        db_session.add(old_email)
        await db_session.commit()

        # Get retention status
        status = await self.retention_enforcer.get_retention_status(connection_id=1)

        # Verify status report
        assert status['has_policy'] is True
        assert status['connection_id'] == 1
        assert 'policy' in status
        assert 'email_stats' in status

        email_stats = status['email_stats']
        assert email_stats['total_emails'] >= 2
        assert email_stats['expired_emails_pending_deletion'] >= 1  # Old email
        assert 'oldest_email' in email_stats
        assert 'newest_email' in email_stats