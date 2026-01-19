"""Participant email and name normalization for Gmail integration."""

import re
import logging
from typing import Dict, List, Optional, Tuple, Set
from dataclasses import dataclass
from email.header import decode_header
from email.utils import parseaddr, getaddresses


logger = logging.getLogger(__name__)


@dataclass
class EmailParticipant:
    """Normalized email participant information."""

    email: str
    display_name: Optional[str] = None
    normalized_email: Optional[str] = None
    normalized_name: Optional[str] = None
    confidence_score: float = 1.0

    def to_dict(self) -> Dict[str, any]:
        """Convert to dictionary representation."""
        return {
            'email': self.email,
            'display_name': self.display_name,
            'normalized_email': self.normalized_email,
            'normalized_name': self.normalized_name,
            'confidence_score': self.confidence_score
        }


class ParticipantNormalizer:
    """Handles normalization of email participants for entity resolution."""

    def __init__(self):
        """Initialize participant normalizer."""
        # Common name patterns for normalization
        self.name_patterns = {
            # Remove titles and suffixes
            'titles': re.compile(r'\b(Dr|Mr|Ms|Mrs|Miss|Prof|Professor|Sir|Madam)\.?\s*', re.IGNORECASE),
            'suffixes': re.compile(r'\s*(Jr|Sr|III|II|IV|PhD|MD|Esq)\.?\s*$', re.IGNORECASE),

            # Remove organizational suffixes
            'org_suffixes': re.compile(r'\s*\(.*?\)\s*$'),  # Remove parenthetical organizations

            # Extract first and last names
            'first_last': re.compile(r'^([A-Za-z]+)\s+([A-Za-z]+).*$'),

            # Handle "Last, First" format
            'last_first': re.compile(r'^([A-Za-z\s]+),\s*([A-Za-z]+).*$')
        }

        # Domain aliases for email normalization
        self.domain_aliases = {
            'googlemail.com': 'gmail.com',
            'google.com': 'google.com',  # Keep as-is
        }

        # Common email variations
        self.email_patterns = {
            'gmail_dots': re.compile(r'\.(?=.*@gmail\.com)'),  # Remove dots from Gmail addresses
            'plus_addressing': re.compile(r'\+[^@]*(?=@)'),    # Remove plus addressing
        }

    def normalize_email_header(self, header_value: str) -> List[EmailParticipant]:
        """Parse and normalize email header (From, To, Cc, etc.).

        Args:
            header_value: Raw email header value

        Returns:
            List of normalized participants
        """
        participants = []

        try:
            # Decode header if it contains encoded text
            decoded_header = self._decode_email_header(header_value)

            # Parse addresses using email.utils.getaddresses
            addresses = getaddresses([decoded_header])

            for display_name, email_addr in addresses:
                if email_addr:  # Skip empty addresses
                    participant = self._normalize_participant(display_name, email_addr)
                    participants.append(participant)

        except Exception as e:
            logger.warning(f"Error parsing email header '{header_value}': {e}")
            # Fallback: try simple email extraction
            email_match = re.search(r'([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})', header_value)
            if email_match:
                email_addr = email_match.group(1)
                participant = self._normalize_participant("", email_addr)
                participants.append(participant)

        return participants

    def normalize_participant_list(
        self,
        from_addr: Optional[str] = None,
        to_addrs: Optional[str] = None,
        cc_addrs: Optional[str] = None,
        bcc_addrs: Optional[str] = None,
        reply_to: Optional[str] = None
    ) -> Dict[str, List[EmailParticipant]]:
        """Normalize all participants in an email.

        Args:
            from_addr: From header
            to_addrs: To header
            cc_addrs: Cc header
            bcc_addrs: Bcc header
            reply_to: Reply-To header

        Returns:
            Dictionary of participant lists by type
        """
        participants = {
            'from': [],
            'to': [],
            'cc': [],
            'bcc': [],
            'reply_to': []
        }

        if from_addr:
            participants['from'] = self.normalize_email_header(from_addr)

        if to_addrs:
            participants['to'] = self.normalize_email_header(to_addrs)

        if cc_addrs:
            participants['cc'] = self.normalize_email_header(cc_addrs)

        if bcc_addrs:
            participants['bcc'] = self.normalize_email_header(bcc_addrs)

        if reply_to:
            participants['reply_to'] = self.normalize_email_header(reply_to)

        return participants

    def extract_unique_participants(self, participants: Dict[str, List[EmailParticipant]]) -> List[EmailParticipant]:
        """Extract unique participants across all roles.

        Args:
            participants: Participants dictionary by role

        Returns:
            List of unique participants with highest confidence scores
        """
        unique_emails = {}

        for role, participant_list in participants.items():
            for participant in participant_list:
                email_key = participant.normalized_email or participant.email.lower()

                # Keep participant with highest confidence score
                if email_key not in unique_emails or participant.confidence_score > unique_emails[email_key].confidence_score:
                    unique_emails[email_key] = participant

        return list(unique_emails.values())

    def _decode_email_header(self, header_value: str) -> str:
        """Decode RFC 2047 encoded email headers.

        Args:
            header_value: Raw header value

        Returns:
            Decoded string
        """
        try:
            decoded_parts = decode_header(header_value)
            decoded_string = ""

            for part, encoding in decoded_parts:
                if isinstance(part, bytes):
                    # Decode bytes using specified encoding or utf-8 as fallback
                    try:
                        decoded_string += part.decode(encoding or 'utf-8')
                    except (UnicodeDecodeError, LookupError):
                        # Fallback to latin-1 for problematic encodings
                        decoded_string += part.decode('latin-1', errors='replace')
                else:
                    decoded_string += str(part)

            return decoded_string

        except Exception as e:
            logger.warning(f"Error decoding header: {e}")
            return header_value

    def _normalize_participant(self, display_name: str, email_addr: str) -> EmailParticipant:
        """Normalize a single participant.

        Args:
            display_name: Display name from email header
            email_addr: Email address

        Returns:
            Normalized participant
        """
        # Clean and normalize email address
        normalized_email = self._normalize_email(email_addr)

        # Clean and normalize display name
        normalized_name = self._normalize_display_name(display_name) if display_name else None

        # Calculate confidence score
        confidence = self._calculate_confidence(display_name, email_addr, normalized_email, normalized_name)

        return EmailParticipant(
            email=email_addr.strip(),
            display_name=display_name.strip() if display_name else None,
            normalized_email=normalized_email,
            normalized_name=normalized_name,
            confidence_score=confidence
        )

    def _normalize_email(self, email_addr: str) -> str:
        """Normalize email address for entity resolution.

        Args:
            email_addr: Raw email address

        Returns:
            Normalized email address
        """
        email = email_addr.lower().strip()

        # Remove plus addressing (user+tag@domain.com -> user@domain.com)
        email = self.email_patterns['plus_addressing'].sub('', email)

        # Handle Gmail-specific normalization
        if '@gmail.com' in email or '@googlemail.com' in email:
            # Remove dots from Gmail local part
            local, domain = email.split('@', 1)
            local = local.replace('.', '')

            # Normalize googlemail.com to gmail.com
            if domain == 'googlemail.com':
                domain = 'gmail.com'

            email = f"{local}@{domain}"

        # Apply domain aliases
        for alias, canonical in self.domain_aliases.items():
            if email.endswith(f"@{alias}"):
                email = email.replace(f"@{alias}", f"@{canonical}")
                break

        return email

    def _normalize_display_name(self, display_name: str) -> Optional[str]:
        """Normalize display name for entity resolution.

        Args:
            display_name: Raw display name

        Returns:
            Normalized display name
        """
        if not display_name or not display_name.strip():
            return None

        name = display_name.strip()

        # Remove quotes if the entire name is quoted
        if (name.startswith('"') and name.endswith('"')) or (name.startswith("'") and name.endswith("'")):
            name = name[1:-1].strip()

        # Remove organizational suffixes in parentheses
        name = self.name_patterns['org_suffixes'].sub('', name)

        # Remove titles
        name = self.name_patterns['titles'].sub('', name)

        # Remove suffixes
        name = self.name_patterns['suffixes'].sub('', name)

        # Clean up whitespace
        name = re.sub(r'\s+', ' ', name).strip()

        # If it looks like "Last, First" format, normalize to "First Last"
        last_first_match = self.name_patterns['last_first'].match(name)
        if last_first_match:
            last_name, first_name = last_first_match.groups()
            name = f"{first_name.strip()} {last_name.strip()}"

        # Ensure we have a valid name
        if len(name) < 2 or not re.search(r'[A-Za-z]', name):
            return None

        # Convert to title case while preserving known patterns
        return self._smart_title_case(name)

    def _smart_title_case(self, name: str) -> str:
        """Apply intelligent title casing to names.

        Args:
            name: Name to title case

        Returns:
            Title cased name
        """
        # Split by spaces and title case each part
        parts = []
        for part in name.split():
            # Handle special cases like "von", "de", "la", etc.
            if part.lower() in ['von', 'van', 'de', 'la', 'le', 'di', 'da', 'del', 'della', 'des', 'du', 'el']:
                parts.append(part.lower())
            # Handle hyphenated names
            elif '-' in part:
                hyphenated_parts = []
                for subpart in part.split('-'):
                    if subpart:
                        hyphenated_parts.append(subpart.capitalize())
                parts.append('-'.join(hyphenated_parts))
            # Regular name part
            else:
                parts.append(part.capitalize())

        return ' '.join(parts)

    def _calculate_confidence(
        self,
        display_name: str,
        email_addr: str,
        normalized_email: str,
        normalized_name: Optional[str]
    ) -> float:
        """Calculate confidence score for normalization quality.

        Args:
            display_name: Original display name
            email_addr: Original email address
            normalized_email: Normalized email
            normalized_name: Normalized name

        Returns:
            Confidence score between 0 and 1
        """
        confidence = 1.0

        # Reduce confidence if email normalization made significant changes
        if email_addr.lower() != normalized_email:
            confidence *= 0.95

        # Reduce confidence if display name normalization made significant changes
        if display_name and normalized_name:
            if len(display_name) - len(normalized_name) > 10:  # Significant length reduction
                confidence *= 0.9

        # Increase confidence if we have both email and name
        if normalized_name and normalized_email:
            confidence *= 1.05

        # Reduce confidence if name is very short
        if normalized_name and len(normalized_name) < 3:
            confidence *= 0.8

        # Reduce confidence for suspicious patterns
        if email_addr and ('noreply' in email_addr.lower() or 'no-reply' in email_addr.lower()):
            confidence *= 0.7

        return min(1.0, confidence)  # Cap at 1.0


def normalize_participants_from_gmail_message(gmail_message: Dict) -> Dict[str, any]:
    """Extract and normalize participants from Gmail message data.

    Args:
        gmail_message: Gmail message dictionary from API

    Returns:
        Dictionary with normalized participant information
    """
    normalizer = ParticipantNormalizer()

    # Extract headers
    headers = {header['name'].lower(): header['value'] for header in gmail_message.get('payload', {}).get('headers', [])}

    # Normalize participants
    participants = normalizer.normalize_participant_list(
        from_addr=headers.get('from'),
        to_addrs=headers.get('to'),
        cc_addrs=headers.get('cc'),
        bcc_addrs=headers.get('bcc'),
        reply_to=headers.get('reply-to')
    )

    # Get unique participants for entity resolution
    unique_participants = normalizer.extract_unique_participants(participants)

    return {
        'participants_by_role': {
            role: [p.to_dict() for p in participant_list]
            for role, participant_list in participants.items()
        },
        'unique_participants': [p.to_dict() for p in unique_participants],
        'participant_count': len(unique_participants),
        'has_external_participants': any(
            not p.normalized_email.endswith(('@gmail.com', '@google.com'))
            for p in unique_participants
        )
    }