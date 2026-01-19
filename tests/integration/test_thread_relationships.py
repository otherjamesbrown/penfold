"""Integration tests for email thread relationship mapping."""

import pytest
from unittest.mock import Mock, AsyncMock, patch
from datetime import datetime
import asyncio

from penf_lib.connectors.gmail.metadata import GmailMetadataExtractor, ThreadRelationship
from penf_lib.connectors.gmail.participants import normalize_participants_from_gmail_message


class TestThreadRelationshipMapping:
    """Test thread relationship mapping functionality."""

    def setup_method(self):
        """Set up test fixtures."""
        self.extractor = GmailMetadataExtractor()

    def create_message_in_thread(self, message_id, thread_id, references=None, in_reply_to=None, subject="Test Thread"):
        """Create a Gmail message that's part of a thread."""
        headers = [
            {'name': 'Message-ID', 'value': f'<{message_id}@example.com>'},
            {'name': 'From', 'value': 'sender@example.com'},
            {'name': 'To', 'value': 'recipient@example.com'},
            {'name': 'Subject', 'value': subject},
            {'name': 'Date', 'value': 'Sat, 13 Jan 2024 10:00:00 -0800'},
        ]

        if in_reply_to:
            headers.append({'name': 'In-Reply-To', 'value': f'<{in_reply_to}@example.com>'})

        if references:
            ref_string = ' '.join([f'<{ref}@example.com>' for ref in references])
            headers.append({'name': 'References', 'value': ref_string})

        return {
            'id': message_id,
            'threadId': thread_id,
            'labelIds': ['INBOX'],
            'sizeEstimate': 1024,
            'internalDate': '1705140000000',
            'payload': {
                'mimeType': 'text/plain',
                'headers': headers,
                'body': {'data': 'VGVzdCBjb250ZW50'}
            }
        }

    def test_root_message_identification(self):
        """Test identifying root message in thread."""
        # Root message has no references or in-reply-to
        root_message = self.create_message_in_thread('root_001', 'thread_001')
        metadata = self.extractor.extract_metadata(root_message)

        thread_info = metadata.thread_info
        assert thread_info.is_root_message
        assert thread_info.reply_depth == 0
        assert thread_info.parent_message_id is None
        assert len(thread_info.references) == 0

    def test_reply_message_identification(self):
        """Test identifying reply message in thread."""
        # Reply message with in-reply-to and references
        reply_message = self.create_message_in_thread(
            'reply_001', 'thread_001',
            references=['root_001'],
            in_reply_to='root_001'
        )
        metadata = self.extractor.extract_metadata(reply_message)

        thread_info = metadata.thread_info
        assert not thread_info.is_root_message
        assert thread_info.reply_depth == 1
        assert thread_info.parent_message_id == 'root_001@example.com'
        assert thread_info.in_reply_to == 'root_001@example.com'
        assert 'root_001@example.com' in thread_info.references

    def test_nested_reply_chain(self):
        """Test deep reply chain with multiple references."""
        # Deep reply with multiple references
        deep_reply = self.create_message_in_thread(
            'reply_003', 'thread_001',
            references=['root_001', 'reply_001', 'reply_002'],
            in_reply_to='reply_002'
        )
        metadata = self.extractor.extract_metadata(deep_reply)

        thread_info = metadata.thread_info
        assert not thread_info.is_root_message
        assert thread_info.reply_depth == 3  # Number of references
        assert thread_info.parent_message_id == 'reply_002@example.com'
        assert len(thread_info.references) == 3

    def test_subject_line_threading(self):
        """Test subject-based thread detection."""
        # Messages with Re: prefix should have normalized subjects
        original = self.create_message_in_thread('msg_001', 'thread_001', subject="Project Update")
        reply = self.create_message_in_thread('msg_002', 'thread_001', subject="Re: Project Update")
        forward = self.create_message_in_thread('msg_003', 'thread_001', subject="Fwd: Re: Project Update")

        original_meta = self.extractor.extract_metadata(original)
        reply_meta = self.extractor.extract_metadata(reply)
        forward_meta = self.extractor.extract_metadata(forward)

        # All should have same normalized subject and subject hash
        assert original_meta.normalized_subject == "Project Update"
        assert reply_meta.normalized_subject == "Project Update"
        assert forward_meta.normalized_subject == "Project Update"

        assert original_meta.subject_hash == reply_meta.subject_hash == forward_meta.subject_hash

    def test_complex_subject_normalization(self):
        """Test complex subject normalization patterns."""
        subjects = [
            ("Re: Re: Original Subject", "Original Subject"),
            ("Fwd: Meeting Notes [TEAM]", "Meeting Notes"),
            ("RE: Project Update (URGENT)", "Project Update (URGENT)"),
            ("Re: Fw: Important Info", "Important Info"),
        ]

        for raw_subject, expected_normalized in subjects:
            message = self.create_message_in_thread('test_msg', 'test_thread', subject=raw_subject)
            metadata = self.extractor.extract_metadata(message)
            assert metadata.normalized_subject == expected_normalized

    def test_participant_relationship_mapping(self):
        """Test mapping participants across thread messages."""
        # Create a conversation between multiple participants
        messages = [
            # Root message from Alice
            self.create_message_with_participants(
                'msg_001', 'thread_001',
                from_addr='Alice <alice@company.com>',
                to_addr='Bob <bob@company.com>, Charlie <charlie@company.com>'
            ),
            # Reply from Bob
            self.create_message_with_participants(
                'msg_002', 'thread_001',
                from_addr='Bob <bob@company.com>',
                to_addr='Alice <alice@company.com>',
                cc_addr='Charlie <charlie@company.com>',
                in_reply_to='msg_001'
            ),
            # Reply from Charlie
            self.create_message_with_participants(
                'msg_003', 'thread_001',
                from_addr='Charlie <charlie@company.com>',
                to_addr='Alice <alice@company.com>, Bob <bob@company.com>',
                in_reply_to='msg_002'
            )
        ]

        # Extract participants from all messages
        all_participants = set()
        for message in messages:
            metadata = self.extractor.extract_metadata(message)
            participants = metadata.participants['unique_participants']
            for participant in participants:
                all_participants.add(participant['normalized_email'])

        # Should find all three participants
        expected_participants = {'alice@company.com', 'bob@company.com', 'charlie@company.com'}
        assert all_participants == expected_participants

    def create_message_with_participants(self, message_id, thread_id, from_addr, to_addr, cc_addr=None, in_reply_to=None):
        """Create a message with specific participant configuration."""
        headers = [
            {'name': 'Message-ID', 'value': f'<{message_id}@example.com>'},
            {'name': 'From', 'value': from_addr},
            {'name': 'To', 'value': to_addr},
            {'name': 'Subject', 'value': 'Thread Subject'},
            {'name': 'Date', 'value': 'Sat, 13 Jan 2024 10:00:00 -0800'},
        ]

        if cc_addr:
            headers.append({'name': 'Cc', 'value': cc_addr})

        if in_reply_to:
            headers.append({'name': 'In-Reply-To', 'value': f'<{in_reply_to}@example.com>'})
            headers.append({'name': 'References', 'value': f'<{in_reply_to}@example.com>'})

        return {
            'id': message_id,
            'threadId': thread_id,
            'labelIds': ['INBOX'],
            'sizeEstimate': 1024,
            'internalDate': '1705140000000',
            'payload': {
                'mimeType': 'text/plain',
                'headers': headers,
                'body': {'data': 'VGVzdCBjb250ZW50'}
            }
        }

    def test_conversation_flow_analysis(self):
        """Test analyzing conversation flow and participant patterns."""
        # Create a multi-participant conversation
        conversation = [
            # Alice starts thread
            ('msg_001', 'Alice <alice@company.com>', 'team@company.com', None),
            # Bob responds to Alice
            ('msg_002', 'Bob <bob@company.com>', 'Alice <alice@company.com>', 'msg_001'),
            # Alice responds to Bob
            ('msg_003', 'Alice <alice@company.com>', 'Bob <bob@company.com>', 'msg_002'),
            # Charlie joins conversation
            ('msg_004', 'Charlie <charlie@company.com>', 'Alice <alice@company.com>, Bob <bob@company.com>', 'msg_003'),
        ]

        conversation_metadata = []
        for msg_id, from_addr, to_addr, reply_to in conversation:
            message = self.create_message_with_participants(msg_id, 'thread_001', from_addr, to_addr, in_reply_to=reply_to)
            metadata = self.extractor.extract_metadata(message)
            conversation_metadata.append(metadata)

        # Analyze conversation patterns
        reply_depths = [meta.thread_info.reply_depth for meta in conversation_metadata]
        assert reply_depths == [0, 1, 2, 3]  # Increasing reply depth

        is_root_flags = [meta.thread_info.is_root_message for meta in conversation_metadata]
        assert is_root_flags == [True, False, False, False]  # Only first is root

        # Check participant evolution
        participant_counts = [meta.participants['participant_count'] for meta in conversation_metadata]
        # Should show increasing or varying participant involvement

    def test_thread_relationship_consistency(self):
        """Test that thread relationships are consistent across processing."""
        # Create the same message multiple times and ensure consistent results
        message_data = self.create_message_in_thread(
            'consistent_test', 'thread_test',
            references=['root_msg', 'reply_1'],
            in_reply_to='reply_1'
        )

        # Extract metadata multiple times
        results = []
        for _ in range(5):
            metadata = self.extractor.extract_metadata(message_data)
            results.append({
                'is_root': metadata.thread_info.is_root_message,
                'reply_depth': metadata.thread_info.reply_depth,
                'parent_id': metadata.thread_info.parent_message_id,
                'subject_hash': metadata.subject_hash
            })

        # All results should be identical
        first_result = results[0]
        for result in results[1:]:
            assert result == first_result

    def test_malformed_thread_headers(self):
        """Test handling of malformed or missing thread headers."""
        # Message with malformed References header
        malformed_message = {
            'id': 'malformed_001',
            'threadId': 'thread_001',
            'labelIds': ['INBOX'],
            'sizeEstimate': 1024,
            'internalDate': '1705140000000',
            'payload': {
                'mimeType': 'text/plain',
                'headers': [
                    {'name': 'From', 'value': 'test@example.com'},
                    {'name': 'To', 'value': 'recipient@example.com'},
                    {'name': 'Subject', 'value': 'Test'},
                    {'name': 'References', 'value': 'malformed-reference-without-brackets'},
                    {'name': 'In-Reply-To', 'value': ''},  # Empty in-reply-to
                ]
            }
        }

        # Should handle gracefully without errors
        metadata = self.extractor.extract_metadata(malformed_message)
        thread_info = metadata.thread_info

        # Should default to root message when headers are malformed
        assert thread_info.is_root_message
        assert thread_info.reply_depth == 0

    @pytest.mark.asyncio
    async def test_thread_relationship_performance(self):
        """Test performance of thread relationship extraction."""
        import time

        # Create a large number of messages
        messages = []
        for i in range(100):
            message = self.create_message_in_thread(
                f'perf_test_{i}', 'thread_perf',
                references=[f'msg_{j}' for j in range(i)] if i > 0 else None,
                in_reply_to=f'msg_{i-1}' if i > 0 else None
            )
            messages.append(message)

        # Time the extraction process
        start_time = time.time()

        for message in messages:
            metadata = self.extractor.extract_metadata(message)
            # Verify basic thread info is extracted
            assert metadata.thread_info is not None

        end_time = time.time()
        processing_time = end_time - start_time

        # Should process 100 messages in reasonable time (less than 1 second)
        assert processing_time < 1.0

        # Calculate processing rate
        messages_per_second = len(messages) / processing_time
        assert messages_per_second > 50  # Should process at least 50 messages/second