"""Gmail metadata extraction for enhanced AI processing."""

import re
import logging
from datetime import datetime
from typing import Dict, List, Optional, Any, Tuple, Set
from dataclasses import dataclass, asdict
from email.utils import parsedate_to_datetime, parseaddr
import hashlib
import uuid

from .participants import normalize_participants_from_gmail_message


logger = logging.getLogger(__name__)


@dataclass
class ThreadRelationship:
    """Represents a relationship between messages in a thread."""

    parent_message_id: Optional[str]
    references: List[str]
    in_reply_to: Optional[str]
    position_in_thread: int
    is_root_message: bool
    reply_depth: int


@dataclass
class MessagePriority:
    """Priority and importance indicators."""

    importance_level: str  # high, normal, low
    priority_score: float  # 0-1 scale
    urgency_indicators: List[str]
    is_automated: bool
    confidence_score: float


@dataclass
class ContentStructure:
    """Information about message content structure."""

    has_plain_text: bool
    has_html: bool
    has_attachments: bool
    is_multipart: bool
    content_types: List[str]
    message_format: str  # plain, html, multipart, rich
    encoding: Optional[str]


@dataclass
class ExtractedMetadata:
    """Comprehensive metadata extracted from Gmail message."""

    # Message identification
    gmail_message_id: str
    gmail_thread_id: str
    message_id_header: Optional[str]

    # Thread relationships
    thread_info: ThreadRelationship

    # Participants
    participants: Dict[str, Any]

    # Content structure
    content_structure: ContentStructure

    # Priority and importance
    priority: MessagePriority

    # Gmail-specific metadata
    gmail_labels: List[str]
    is_unread: bool
    is_starred: bool
    is_important: bool
    size_estimate: int

    # Temporal information
    internal_date: datetime
    sent_date: Optional[datetime]
    received_date: Optional[datetime]

    # Security and filtering
    spam_score: Optional[float]
    virus_scan_result: Optional[str]
    authentication_results: Dict[str, Any]

    # Subject and threading
    subject: Optional[str]
    normalized_subject: Optional[str]
    subject_hash: str

    # Additional headers
    custom_headers: Dict[str, str]
    delivery_info: Dict[str, Any]

    def to_dict(self) -> Dict[str, Any]:
        """Convert to dictionary representation."""
        return asdict(self)


class GmailMetadataExtractor:
    """Extracts rich metadata from Gmail messages."""

    def __init__(self):
        """Initialize metadata extractor."""
        # Subject normalization patterns
        self.subject_patterns = {
            'reply_forward': re.compile(r'^(Re|Fwd|Fw):\s*', re.IGNORECASE),
            'multiple_re': re.compile(r'^(Re:\s*)+(.*)', re.IGNORECASE),
            'brackets': re.compile(r'\[[^\]]*\]'),  # Remove bracketed content
            'parentheses': re.compile(r'\([^)]*\)'),  # Remove parenthetical content
        }

        # Priority detection patterns
        self.priority_patterns = {
            'urgent_words': re.compile(r'\b(urgent|asap|rush|emergency|critical|immediate)\b', re.IGNORECASE),
            'deadline_words': re.compile(r'\b(deadline|due|expires|time.?sensitive)\b', re.IGNORECASE),
            'action_words': re.compile(r'\b(action.?required|please.?respond|need.?feedback)\b', re.IGNORECASE),
            'meeting_words': re.compile(r'\b(meeting|call|zoom|webex|conference)\b', re.IGNORECASE),
        }

        # Automated message patterns
        self.automated_patterns = [
            re.compile(r'\b(noreply|no-reply|donotreply|automated)\b', re.IGNORECASE),
            re.compile(r'\b(newsletter|notification|alert|digest)\b', re.IGNORECASE),
            re.compile(r'\b(calendar|reminder|invoice|receipt)\b', re.IGNORECASE),
        ]

        # Content type mappings
        self.content_type_map = {
            'text/plain': 'plain',
            'text/html': 'html',
            'multipart/alternative': 'multipart',
            'multipart/mixed': 'multipart',
            'multipart/related': 'rich',
        }

    def extract_metadata(self, gmail_message: Dict[str, Any]) -> ExtractedMetadata:
        """Extract comprehensive metadata from Gmail message.

        Args:
            gmail_message: Gmail message data from API

        Returns:
            Extracted metadata object
        """
        logger.debug(f"Extracting metadata for message {gmail_message.get('id', 'unknown')}")

        # Extract basic information
        message_id = gmail_message['id']
        thread_id = gmail_message['threadId']
        size_estimate = gmail_message.get('sizeEstimate', 0)

        # Extract payload information
        payload = gmail_message.get('payload', {})
        headers = self._parse_headers(payload.get('headers', []))

        # Extract thread relationships
        thread_info = self._extract_thread_relationships(headers, payload)

        # Extract participants
        participants = normalize_participants_from_gmail_message(gmail_message)

        # Extract content structure
        content_structure = self._extract_content_structure(payload)

        # Extract priority information
        priority = self._extract_priority(headers, content_structure)

        # Process Gmail labels
        gmail_labels = gmail_message.get('labelIds', [])
        is_unread = 'UNREAD' in gmail_labels
        is_starred = 'STARRED' in gmail_labels
        is_important = 'IMPORTANT' in gmail_labels or 'CATEGORY_PERSONAL' in gmail_labels

        # Extract temporal information
        internal_date = self._parse_gmail_date(gmail_message.get('internalDate'))
        sent_date = self._parse_header_date(headers.get('date'))
        received_date = internal_date  # Use internal date as received date

        # Extract security information
        spam_score = self._calculate_spam_score(headers, gmail_labels)
        auth_results = self._extract_authentication_results(headers)

        # Process subject
        subject = headers.get('subject', '').strip()
        normalized_subject = self._normalize_subject(subject)
        subject_hash = self._calculate_subject_hash(normalized_subject)

        # Extract additional metadata
        custom_headers = self._extract_custom_headers(headers)
        delivery_info = self._extract_delivery_info(headers)

        return ExtractedMetadata(
            gmail_message_id=message_id,
            gmail_thread_id=thread_id,
            message_id_header=headers.get('message-id'),
            thread_info=thread_info,
            participants=participants,
            content_structure=content_structure,
            priority=priority,
            gmail_labels=gmail_labels,
            is_unread=is_unread,
            is_starred=is_starred,
            is_important=is_important,
            size_estimate=size_estimate,
            internal_date=internal_date,
            sent_date=sent_date,
            received_date=received_date,
            spam_score=spam_score,
            virus_scan_result=None,  # Not available from Gmail API
            authentication_results=auth_results,
            subject=subject,
            normalized_subject=normalized_subject,
            subject_hash=subject_hash,
            custom_headers=custom_headers,
            delivery_info=delivery_info
        )

    def _parse_headers(self, headers_list: List[Dict[str, str]]) -> Dict[str, str]:
        """Parse headers list into dictionary.

        Args:
            headers_list: List of header dictionaries from Gmail API

        Returns:
            Dictionary of headers with lowercase names
        """
        headers = {}
        for header in headers_list:
            name = header.get('name', '').lower()
            value = header.get('value', '')
            headers[name] = value
        return headers

    def _extract_thread_relationships(self, headers: Dict[str, str], payload: Dict) -> ThreadRelationship:
        """Extract thread relationship information.

        Args:
            headers: Parsed email headers
            payload: Gmail message payload

        Returns:
            Thread relationship information
        """
        # Extract reference headers
        references = []
        references_header = headers.get('references', '')
        if references_header:
            # References are space-separated message IDs in angle brackets
            references = re.findall(r'<([^>]+)>', references_header)

        in_reply_to = headers.get('in-reply-to')
        if in_reply_to:
            # Clean angle brackets
            in_reply_to = in_reply_to.strip('<>')

        # Determine position in thread
        is_root_message = not bool(references or in_reply_to)
        reply_depth = len(references)

        # Parent is the most recent reference or in-reply-to
        parent_message_id = in_reply_to if in_reply_to else (references[-1] if references else None)

        return ThreadRelationship(
            parent_message_id=parent_message_id,
            references=references,
            in_reply_to=in_reply_to,
            position_in_thread=0,  # Will be updated when processing full thread
            is_root_message=is_root_message,
            reply_depth=reply_depth
        )

    def _extract_content_structure(self, payload: Dict) -> ContentStructure:
        """Extract content structure information.

        Args:
            payload: Gmail message payload

        Returns:
            Content structure information
        """
        mime_type = payload.get('mimeType', 'unknown')
        parts = payload.get('parts', [])

        has_plain_text = False
        has_html = False
        has_attachments = False
        content_types = []

        # Check main content type
        content_types.append(mime_type)

        # Analyze parts for multipart messages
        if parts:
            for part in parts:
                part_mime = part.get('mimeType', '')
                content_types.append(part_mime)

                if part_mime == 'text/plain':
                    has_plain_text = True
                elif part_mime == 'text/html':
                    has_html = True
                elif part.get('filename') or part_mime.startswith('image/') or part_mime.startswith('application/'):
                    has_attachments = True

                # Recursively check nested parts
                nested_parts = part.get('parts', [])
                if nested_parts:
                    for nested_part in nested_parts:
                        nested_mime = nested_part.get('mimeType', '')
                        content_types.append(nested_mime)
                        if nested_mime == 'text/plain':
                            has_plain_text = True
                        elif nested_mime == 'text/html':
                            has_html = True
        else:
            # Single part message
            if mime_type == 'text/plain':
                has_plain_text = True
            elif mime_type == 'text/html':
                has_html = True

        # Determine overall format
        is_multipart = mime_type.startswith('multipart/')
        if is_multipart and has_attachments:
            message_format = 'rich'
        elif is_multipart:
            message_format = 'multipart'
        elif has_html:
            message_format = 'html'
        else:
            message_format = 'plain'

        # Detect encoding
        encoding = None
        body = payload.get('body', {})
        if body and 'data' in body:
            encoding = 'base64'  # Gmail API returns base64 encoded content

        return ContentStructure(
            has_plain_text=has_plain_text,
            has_html=has_html,
            has_attachments=has_attachments,
            is_multipart=is_multipart,
            content_types=list(set(content_types)),  # Remove duplicates
            message_format=message_format,
            encoding=encoding
        )

    def _extract_priority(self, headers: Dict[str, str], content_structure: ContentStructure) -> MessagePriority:
        """Extract priority and importance indicators.

        Args:
            headers: Email headers
            content_structure: Content structure information

        Returns:
            Priority information
        """
        priority_score = 0.5  # Default normal priority
        importance_level = 'normal'
        urgency_indicators = []
        confidence_score = 0.7

        # Check explicit priority headers
        x_priority = headers.get('x-priority', '').strip()
        x_msmail_priority = headers.get('x-msmail-priority', '').strip()
        importance_header = headers.get('importance', '').strip()

        if x_priority in ['1', '2']:
            priority_score = 0.8
            importance_level = 'high'
            urgency_indicators.append('high_priority_header')
            confidence_score = 0.9
        elif x_priority in ['4', '5']:
            priority_score = 0.2
            importance_level = 'low'
            confidence_score = 0.9

        if importance_header.lower() in ['high', 'urgent']:
            priority_score = max(priority_score, 0.8)
            importance_level = 'high'
            urgency_indicators.append('importance_header')

        # Analyze subject for urgency indicators
        subject = headers.get('subject', '').lower()
        for pattern_name, pattern in self.priority_patterns.items():
            if pattern.search(subject):
                urgency_indicators.append(f'subject_{pattern_name}')
                if pattern_name == 'urgent_words':
                    priority_score = max(priority_score, 0.8)
                elif pattern_name in ['deadline_words', 'action_words']:
                    priority_score = max(priority_score, 0.7)

        # Check for automated messages
        is_automated = False
        from_addr = headers.get('from', '').lower()
        for pattern in self.automated_patterns:
            if pattern.search(from_addr) or pattern.search(subject):
                is_automated = True
                priority_score = min(priority_score, 0.3)  # Lower priority for automated
                break

        # Adjust based on content structure
        if content_structure.has_attachments:
            priority_score += 0.1  # Slightly higher for attachments

        # Final importance level determination
        if priority_score >= 0.7:
            importance_level = 'high'
        elif priority_score <= 0.3:
            importance_level = 'low'

        return MessagePriority(
            importance_level=importance_level,
            priority_score=min(1.0, priority_score),
            urgency_indicators=urgency_indicators,
            is_automated=is_automated,
            confidence_score=confidence_score
        )

    def _parse_gmail_date(self, internal_date_str: str) -> datetime:
        """Parse Gmail internal date string.

        Args:
            internal_date_str: Gmail internal date (milliseconds since epoch)

        Returns:
            Parsed datetime
        """
        try:
            # Gmail internal date is milliseconds since Unix epoch
            timestamp_ms = int(internal_date_str)
            return datetime.fromtimestamp(timestamp_ms / 1000)
        except (ValueError, TypeError):
            logger.warning(f"Could not parse Gmail internal date: {internal_date_str}")
            return datetime.utcnow()

    def _parse_header_date(self, date_header: Optional[str]) -> Optional[datetime]:
        """Parse email Date header.

        Args:
            date_header: Date header value

        Returns:
            Parsed datetime or None
        """
        if not date_header:
            return None

        try:
            return parsedate_to_datetime(date_header)
        except Exception as e:
            logger.warning(f"Could not parse date header '{date_header}': {e}")
            return None

    def _calculate_spam_score(self, headers: Dict[str, str], gmail_labels: List[str]) -> Optional[float]:
        """Calculate spam score based on headers and labels.

        Args:
            headers: Email headers
            gmail_labels: Gmail labels

        Returns:
            Spam score between 0 and 1, or None if not determinable
        """
        if 'SPAM' in gmail_labels:
            return 0.9

        spam_score = 0.0

        # Check authentication headers
        spf = headers.get('received-spf', '').lower()
        if 'fail' in spf:
            spam_score += 0.3

        dkim = headers.get('dkim-signature')
        if not dkim:
            spam_score += 0.1

        # Check for suspicious patterns
        from_addr = headers.get('from', '')
        if re.search(r'[0-9]{3,}@', from_addr):  # Numeric sender
            spam_score += 0.2

        return min(1.0, spam_score) if spam_score > 0 else None

    def _extract_authentication_results(self, headers: Dict[str, str]) -> Dict[str, Any]:
        """Extract email authentication results.

        Args:
            headers: Email headers

        Returns:
            Authentication results
        """
        auth_results = {}

        # SPF
        spf = headers.get('received-spf', '')
        if spf:
            auth_results['spf'] = spf.split()[0].lower() if spf.split() else 'unknown'

        # DKIM
        dkim = headers.get('dkim-signature')
        auth_results['has_dkim'] = bool(dkim)

        # DMARC
        auth_results_header = headers.get('authentication-results', '')
        if 'dmarc=' in auth_results_header.lower():
            dmarc_match = re.search(r'dmarc=(\w+)', auth_results_header.lower())
            if dmarc_match:
                auth_results['dmarc'] = dmarc_match.group(1)

        return auth_results

    def _normalize_subject(self, subject: str) -> str:
        """Normalize subject for threading and analysis.

        Args:
            subject: Raw subject line

        Returns:
            Normalized subject
        """
        if not subject:
            return ''

        normalized = subject.strip()

        # Remove reply/forward prefixes
        normalized = self.subject_patterns['reply_forward'].sub('', normalized)

        # Handle multiple "Re:" prefixes
        multiple_re_match = self.subject_patterns['multiple_re'].match(normalized)
        if multiple_re_match:
            normalized = multiple_re_match.group(2)

        # Remove common bracketed content (but preserve important brackets)
        # This is conservative - only remove obvious list/system tags
        normalized = re.sub(r'\[[A-Z]{2,}\]', '', normalized)  # Remove all-caps tags like [JIRA]

        # Clean up whitespace
        normalized = re.sub(r'\s+', ' ', normalized).strip()

        return normalized

    def _calculate_subject_hash(self, normalized_subject: str) -> str:
        """Calculate hash for subject-based threading.

        Args:
            normalized_subject: Normalized subject line

        Returns:
            Subject hash for threading
        """
        if not normalized_subject:
            return ''

        # Create a stable hash for subject-based threading
        subject_bytes = normalized_subject.lower().encode('utf-8')
        return hashlib.md5(subject_bytes).hexdigest()[:16]

    def _extract_custom_headers(self, headers: Dict[str, str]) -> Dict[str, str]:
        """Extract non-standard headers that might be useful.

        Args:
            headers: All email headers

        Returns:
            Dictionary of custom headers
        """
        standard_headers = {
            'from', 'to', 'cc', 'bcc', 'subject', 'date', 'message-id',
            'in-reply-to', 'references', 'content-type', 'mime-version',
            'received', 'return-path', 'reply-to'
        }

        custom_headers = {}
        for name, value in headers.items():
            if name not in standard_headers and (name.startswith('x-') or name.startswith('list-')):
                custom_headers[name] = value

        return custom_headers

    def _extract_delivery_info(self, headers: Dict[str, str]) -> Dict[str, Any]:
        """Extract message delivery information.

        Args:
            headers: Email headers

        Returns:
            Delivery information
        """
        delivery_info = {}

        # Extract list information
        list_id = headers.get('list-id')
        if list_id:
            delivery_info['list_id'] = list_id
            delivery_info['is_mailing_list'] = True

        list_unsubscribe = headers.get('list-unsubscribe')
        if list_unsubscribe:
            delivery_info['unsubscribe_info'] = list_unsubscribe

        # Extract auto-submitted header
        auto_submitted = headers.get('auto-submitted')
        if auto_submitted:
            delivery_info['auto_submitted'] = auto_submitted

        # Extract precedence
        precedence = headers.get('precedence')
        if precedence:
            delivery_info['precedence'] = precedence.lower()

        return delivery_info