"""Gmail privacy and security controls for email processing."""

import re
import logging
import time
from datetime import datetime, timedelta
from typing import List, Dict, Any, Optional, Tuple, Set
import asyncio

from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, and_, or_, delete, func, desc
from sqlalchemy.orm import selectinload

from ...models.privacy_filter import (
    PrivacyFilter,
    PrivacyAuditLog,
    RetentionPolicy,
    FilterType,
    FilterAction
)
from ...models.email_message import EmailMessage, EmailThread, EmailAttachment
from ...storage.database import db_manager
from .metadata import ExtractedMetadata


logger = logging.getLogger(__name__)


class PrivacyFilterResult:
    """Result of privacy filter evaluation."""

    def __init__(
        self,
        should_exclude: bool = False,
        action: FilterAction = FilterAction.EXCLUDE,
        matched_filters: List[Dict[str, Any]] = None,
        processing_notes: List[str] = None,
        confidence_score: int = 100,
        sensitive_content_detected: bool = False
    ):
        """Initialize filter result.

        Args:
            should_exclude: Whether email should be excluded from processing
            action: Action to take based on filters
            matched_filters: List of filters that matched
            processing_notes: Notes about filter processing
            confidence_score: Confidence in filter decision (0-100)
            sensitive_content_detected: Whether sensitive content was detected
        """
        self.should_exclude = should_exclude
        self.action = action
        self.matched_filters = matched_filters or []
        self.processing_notes = processing_notes or []
        self.confidence_score = confidence_score
        self.sensitive_content_detected = sensitive_content_detected


class GmailPrivacyFilter:
    """Privacy filter engine for Gmail email processing."""

    def __init__(self):
        """Initialize privacy filter engine."""
        # Compiled regex cache for performance
        self._regex_cache: Dict[str, re.Pattern] = {}

        # Built-in sensitive content patterns
        self.sensitive_patterns = {
            'ssn': re.compile(r'\b\d{3}-\d{2}-\d{4}\b'),
            'credit_card': re.compile(r'\b\d{4}[- ]?\d{4}[- ]?\d{4}[- ]?\d{4}\b'),
            'phone': re.compile(r'\b\d{3}-\d{3}-\d{4}\b|\(\d{3}\)\s*\d{3}-\d{4}\b'),
            'medical_record': re.compile(r'\bmrn\s*:?\s*\d+\b', re.IGNORECASE),
            'bank_account': re.compile(r'\baccount\s*(?:number|#)?\s*:?\s*\d{8,17}\b', re.IGNORECASE),
            'password': re.compile(r'\bpassword\s*:?\s*\S+\b', re.IGNORECASE),
            'api_key': re.compile(r'\bapi[_\s]*key\s*:?\s*[a-zA-Z0-9]{20,}\b', re.IGNORECASE)
        }

    async def evaluate_email(
        self,
        connection_id: int,
        gmail_message: Dict[str, Any],
        extracted_metadata: ExtractedMetadata
    ) -> PrivacyFilterResult:
        """Evaluate email against privacy filters.

        Args:
            connection_id: Gmail connection ID
            gmail_message: Gmail message data
            extracted_metadata: Extracted metadata

        Returns:
            Privacy filter evaluation result
        """
        start_time = time.time()

        try:
            # Get active filters for connection
            filters = await self._get_active_filters(connection_id)

            if not filters:
                logger.debug(f"No privacy filters configured for connection {connection_id}")
                return PrivacyFilterResult()

            # Evaluate each filter
            matched_filters = []
            processing_notes = []
            highest_action_priority = 0
            final_action = FilterAction.LOG_ONLY
            should_exclude = False

            for filter_obj in filters:
                filter_start = time.time()

                match_result = await self._evaluate_single_filter(
                    filter_obj, gmail_message, extracted_metadata
                )

                filter_time = int((time.time() - filter_start) * 1000)

                if match_result['matched']:
                    matched_filters.append({
                        'filter_id': filter_obj.id,
                        'filter_name': filter_obj.name,
                        'filter_type': filter_obj.filter_type,
                        'action': filter_obj.action,
                        'match_reason': match_result['reason'],
                        'matched_criteria': match_result['criteria'],
                        'processing_time_ms': filter_time,
                        'priority': filter_obj.priority
                    })

                    processing_notes.append(
                        f"Filter '{filter_obj.name}' matched: {match_result['reason']}"
                    )

                    # Update filter statistics
                    await self._update_filter_stats(filter_obj, filter_time)

                    # Determine highest priority action
                    action_priority = self._get_action_priority(FilterAction(filter_obj.action))
                    if action_priority > highest_action_priority:
                        highest_action_priority = action_priority
                        final_action = FilterAction(filter_obj.action)

            # Check for sensitive content
            sensitive_detected = await self._detect_sensitive_content(gmail_message, extracted_metadata)
            if sensitive_detected:
                processing_notes.append("Sensitive content patterns detected")

            # Determine final decision
            if final_action in [FilterAction.EXCLUDE, FilterAction.QUARANTINE]:
                should_exclude = True
            elif sensitive_detected and final_action == FilterAction.LOG_ONLY:
                final_action = FilterAction.MARK_SENSITIVE

            # Log results if any filters matched
            if matched_filters:
                await self._log_privacy_action(
                    connection_id,
                    gmail_message,
                    extracted_metadata,
                    matched_filters,
                    final_action,
                    int((time.time() - start_time) * 1000)
                )

            return PrivacyFilterResult(
                should_exclude=should_exclude,
                action=final_action,
                matched_filters=matched_filters,
                processing_notes=processing_notes,
                confidence_score=95 if matched_filters else 100,
                sensitive_content_detected=sensitive_detected
            )

        except Exception as e:
            logger.error(f"Error evaluating privacy filters: {e}")
            # Fail securely - exclude email if there's an error
            return PrivacyFilterResult(
                should_exclude=True,
                action=FilterAction.EXCLUDE,
                processing_notes=[f"Privacy filter error: {str(e)}"],
                confidence_score=50
            )

    async def _get_active_filters(self, connection_id: int) -> List[PrivacyFilter]:
        """Get active privacy filters for connection.

        Args:
            connection_id: Gmail connection ID

        Returns:
            List of active privacy filters ordered by priority
        """
        async with db_manager.get_session() as session:
            result = await session.execute(
                select(PrivacyFilter)
                .where(
                    and_(
                        PrivacyFilter.connection_id == connection_id,
                        PrivacyFilter.is_active == True
                    )
                )
                .order_by(PrivacyFilter.priority.asc())  # Lower number = higher priority
            )
            return result.scalars().all()

    async def _evaluate_single_filter(
        self,
        filter_obj: PrivacyFilter,
        gmail_message: Dict[str, Any],
        extracted_metadata: ExtractedMetadata
    ) -> Dict[str, Any]:
        """Evaluate a single privacy filter against email.

        Args:
            filter_obj: Privacy filter to evaluate
            gmail_message: Gmail message data
            extracted_metadata: Extracted metadata

        Returns:
            Filter evaluation result
        """
        filter_type = FilterType(filter_obj.filter_type)
        config = filter_obj.filter_config

        try:
            if filter_type == FilterType.LABEL_BASED:
                return await self._evaluate_label_filter(config, gmail_message)
            elif filter_type == FilterType.PARTICIPANT_BASED:
                return await self._evaluate_participant_filter(config, extracted_metadata)
            elif filter_type == FilterType.DOMAIN_BASED:
                return await self._evaluate_domain_filter(config, extracted_metadata)
            elif filter_type == FilterType.CONTENT_PATTERN:
                return await self._evaluate_content_pattern_filter(config, gmail_message, extracted_metadata)
            elif filter_type == FilterType.SENDER_EXCLUSION:
                return await self._evaluate_sender_exclusion_filter(config, extracted_metadata)
            elif filter_type == FilterType.SUBJECT_PATTERN:
                return await self._evaluate_subject_pattern_filter(config, extracted_metadata)
            elif filter_type == FilterType.ATTACHMENT_TYPE:
                return await self._evaluate_attachment_filter(config, gmail_message)
            elif filter_type == FilterType.COMBINED_RULE:
                return await self._evaluate_combined_filter(config, gmail_message, extracted_metadata)
            else:
                logger.warning(f"Unknown filter type: {filter_type}")
                return {'matched': False, 'reason': 'Unknown filter type', 'criteria': {}}

        except Exception as e:
            logger.error(f"Error evaluating filter {filter_obj.id}: {e}")
            return {'matched': False, 'reason': f'Filter evaluation error: {e}', 'criteria': {}}

    async def _evaluate_label_filter(
        self,
        config: Dict[str, Any],
        gmail_message: Dict[str, Any]
    ) -> Dict[str, Any]:
        """Evaluate label-based filter."""
        target_labels = config.get('labels', [])
        match_any = config.get('match_any', True)
        message_labels = gmail_message.get('labelIds', [])

        if match_any:
            # OR logic - any target label found
            matched_labels = [label for label in target_labels if label in message_labels]
            if matched_labels:
                return {
                    'matched': True,
                    'reason': f"Email has filtered labels: {', '.join(matched_labels)}",
                    'criteria': {'matched_labels': matched_labels}
                }
        else:
            # AND logic - all target labels must be present
            missing_labels = [label for label in target_labels if label not in message_labels]
            if not missing_labels:
                return {
                    'matched': True,
                    'reason': f"Email has all required labels: {', '.join(target_labels)}",
                    'criteria': {'required_labels': target_labels}
                }

        return {'matched': False, 'reason': 'No label criteria matched', 'criteria': {}}

    async def _evaluate_participant_filter(
        self,
        config: Dict[str, Any],
        extracted_metadata: ExtractedMetadata
    ) -> Dict[str, Any]:
        """Evaluate participant-based filter."""
        target_participants = config.get('participants', [])
        match_type = config.get('match_type', 'email')
        case_sensitive = config.get('case_sensitive', False)
        match_any = config.get('match_any', True)

        unique_participants = extracted_metadata.participants.get('unique_participants', [])

        matched_participants = []

        for participant in unique_participants:
            participant_value = (
                participant.get('normalized_email', '') if match_type == 'email'
                else participant.get('normalized_name', '')
            )

            if not participant_value:
                continue

            if not case_sensitive:
                participant_value = participant_value.lower()
                comparison_targets = [p.lower() for p in target_participants]
            else:
                comparison_targets = target_participants

            if participant_value in comparison_targets:
                matched_participants.append(participant_value)

        if match_any and matched_participants:
            return {
                'matched': True,
                'reason': f"Filtered {match_type}s found: {', '.join(matched_participants)}",
                'criteria': {'matched_participants': matched_participants}
            }
        elif not match_any and len(matched_participants) == len(target_participants):
            return {
                'matched': True,
                'reason': f"All required {match_type}s present: {', '.join(matched_participants)}",
                'criteria': {'required_participants': matched_participants}
            }

        return {'matched': False, 'reason': 'No participant criteria matched', 'criteria': {}}

    async def _evaluate_domain_filter(
        self,
        config: Dict[str, Any],
        extracted_metadata: ExtractedMetadata
    ) -> Dict[str, Any]:
        """Evaluate domain-based filter."""
        target_domains = config.get('domains', [])
        include_subdomains = config.get('include_subdomains', True)
        match_any = config.get('match_any', True)

        unique_participants = extracted_metadata.participants.get('unique_participants', [])

        matched_domains = []
        participant_emails = []

        for participant in unique_participants:
            email = participant.get('normalized_email', '')
            if '@' in email:
                domain = email.split('@')[1].lower()
                participant_emails.append(email)

                for target_domain in target_domains:
                    target_domain = target_domain.lower()

                    if include_subdomains:
                        if domain == target_domain or domain.endswith('.' + target_domain):
                            matched_domains.append(domain)
                    else:
                        if domain == target_domain:
                            matched_domains.append(domain)

        if match_any and matched_domains:
            return {
                'matched': True,
                'reason': f"Filtered domains found: {', '.join(set(matched_domains))}",
                'criteria': {'matched_domains': list(set(matched_domains))}
            }

        return {'matched': False, 'reason': 'No domain criteria matched', 'criteria': {}}

    async def _evaluate_content_pattern_filter(
        self,
        config: Dict[str, Any],
        gmail_message: Dict[str, Any],
        extracted_metadata: ExtractedMetadata
    ) -> Dict[str, Any]:
        """Evaluate content pattern filter."""
        patterns = config.get('patterns', [])
        match_subject = config.get('match_subject', True)
        match_body = config.get('match_body', True)
        case_sensitive = config.get('case_sensitive', False)
        match_any_pattern = config.get('match_any_pattern', True)

        matched_patterns = []
        match_locations = []

        # Check subject
        if match_subject and extracted_metadata.subject:
            subject = extracted_metadata.subject
            if not case_sensitive:
                subject = subject.lower()

            for pattern in patterns:
                regex_pattern = self._get_compiled_regex(
                    pattern, case_sensitive
                )
                if regex_pattern and regex_pattern.search(subject):
                    matched_patterns.append(pattern)
                    match_locations.append('subject')
                    if match_any_pattern:
                        break

        # Check body content (basic check - would need full body extraction)
        if match_body:
            # For now, check snippet which is available
            snippet = gmail_message.get('snippet', '')
            if snippet:
                if not case_sensitive:
                    snippet = snippet.lower()

                for pattern in patterns:
                    regex_pattern = self._get_compiled_regex(
                        pattern, case_sensitive
                    )
                    if regex_pattern and regex_pattern.search(snippet):
                        matched_patterns.append(pattern)
                        match_locations.append('snippet')
                        if match_any_pattern:
                            break

        if matched_patterns:
            return {
                'matched': True,
                'reason': f"Content patterns matched in {', '.join(set(match_locations))}: {', '.join(set(matched_patterns))}",
                'criteria': {
                    'matched_patterns': list(set(matched_patterns)),
                    'match_locations': list(set(match_locations))
                }
            }

        return {'matched': False, 'reason': 'No content patterns matched', 'criteria': {}}

    async def _evaluate_sender_exclusion_filter(
        self,
        config: Dict[str, Any],
        extracted_metadata: ExtractedMetadata
    ) -> Dict[str, Any]:
        """Evaluate sender exclusion filter."""
        excluded_senders = config.get('excluded_senders', [])
        case_sensitive = config.get('case_sensitive', False)

        from_participants = extracted_metadata.participants.get('participants_by_role', {}).get('from', [])

        if not from_participants:
            return {'matched': False, 'reason': 'No sender information', 'criteria': {}}

        sender = from_participants[0]  # First sender
        sender_email = sender.get('normalized_email', '')
        sender_name = sender.get('normalized_name', '')

        if not case_sensitive:
            sender_email = sender_email.lower()
            sender_name = sender_name.lower() if sender_name else ''
            excluded_senders = [s.lower() for s in excluded_senders]

        if sender_email in excluded_senders or (sender_name and sender_name in excluded_senders):
            return {
                'matched': True,
                'reason': f"Sender excluded: {sender_email}",
                'criteria': {'excluded_sender': sender_email}
            }

        return {'matched': False, 'reason': 'Sender not in exclusion list', 'criteria': {}}

    async def _evaluate_subject_pattern_filter(
        self,
        config: Dict[str, Any],
        extracted_metadata: ExtractedMetadata
    ) -> Dict[str, Any]:
        """Evaluate subject pattern filter."""
        patterns = config.get('patterns', [])
        case_sensitive = config.get('case_sensitive', False)
        match_any_pattern = config.get('match_any_pattern', True)

        if not extracted_metadata.subject:
            return {'matched': False, 'reason': 'No subject to match', 'criteria': {}}

        subject = extracted_metadata.subject
        if not case_sensitive:
            subject = subject.lower()

        matched_patterns = []

        for pattern in patterns:
            regex_pattern = self._get_compiled_regex(pattern, case_sensitive)
            if regex_pattern and regex_pattern.search(subject):
                matched_patterns.append(pattern)
                if match_any_pattern:
                    break

        if matched_patterns:
            return {
                'matched': True,
                'reason': f"Subject patterns matched: {', '.join(matched_patterns)}",
                'criteria': {'matched_patterns': matched_patterns}
            }

        return {'matched': False, 'reason': 'No subject patterns matched', 'criteria': {}}

    async def _evaluate_attachment_filter(
        self,
        config: Dict[str, Any],
        gmail_message: Dict[str, Any]
    ) -> Dict[str, Any]:
        """Evaluate attachment type filter."""
        allowed_types = config.get('allowed_types', [])
        blocked_types = config.get('blocked_types', [])
        max_size_mb = config.get('max_size_mb')

        # Basic attachment check from payload
        payload = gmail_message.get('payload', {})
        parts = payload.get('parts', [])

        attachments_found = []
        for part in parts:
            filename = part.get('filename', '')
            mime_type = part.get('mimeType', '')

            if filename:  # Has filename, likely an attachment
                attachments_found.append({
                    'filename': filename,
                    'mime_type': mime_type,
                    'size': part.get('body', {}).get('size', 0)
                })

        if not attachments_found:
            return {'matched': False, 'reason': 'No attachments found', 'criteria': {}}

        # Check blocked types
        if blocked_types:
            for attachment in attachments_found:
                for blocked_type in blocked_types:
                    if blocked_type.lower() in attachment['mime_type'].lower():
                        return {
                            'matched': True,
                            'reason': f"Blocked attachment type: {attachment['mime_type']}",
                            'criteria': {'blocked_attachment': attachment}
                        }

        # Check allowed types (if specified)
        if allowed_types:
            for attachment in attachments_found:
                type_allowed = any(
                    allowed_type.lower() in attachment['mime_type'].lower()
                    for allowed_type in allowed_types
                )
                if not type_allowed:
                    return {
                        'matched': True,
                        'reason': f"Attachment type not allowed: {attachment['mime_type']}",
                        'criteria': {'disallowed_attachment': attachment}
                    }

        return {'matched': False, 'reason': 'Attachment criteria passed', 'criteria': {}}

    async def _evaluate_combined_filter(
        self,
        config: Dict[str, Any],
        gmail_message: Dict[str, Any],
        extracted_metadata: ExtractedMetadata
    ) -> Dict[str, Any]:
        """Evaluate combined filter with multiple criteria."""
        rules = config.get('rules', [])
        logic = config.get('logic', 'AND')  # AND or OR

        matched_rules = []

        for rule in rules:
            rule_type = rule.get('type')
            rule_config = rule.get('config', {})

            # Recursively evaluate sub-rules
            if rule_type == 'label':
                result = await self._evaluate_label_filter(rule_config, gmail_message)
            elif rule_type == 'participant':
                result = await self._evaluate_participant_filter(rule_config, extracted_metadata)
            elif rule_type == 'domain':
                result = await self._evaluate_domain_filter(rule_config, extracted_metadata)
            # Add other types as needed
            else:
                continue

            if result['matched']:
                matched_rules.append({
                    'rule_type': rule_type,
                    'reason': result['reason']
                })

        # Apply logic
        if logic == 'OR' and matched_rules:
            return {
                'matched': True,
                'reason': f"Combined filter matched (OR logic): {len(matched_rules)} rules",
                'criteria': {'matched_rules': matched_rules}
            }
        elif logic == 'AND' and len(matched_rules) == len(rules):
            return {
                'matched': True,
                'reason': f"Combined filter matched (AND logic): all {len(rules)} rules",
                'criteria': {'matched_rules': matched_rules}
            }

        return {'matched': False, 'reason': 'Combined filter criteria not met', 'criteria': {}}

    async def _detect_sensitive_content(
        self,
        gmail_message: Dict[str, Any],
        extracted_metadata: ExtractedMetadata
    ) -> bool:
        """Detect sensitive content patterns in email.

        Args:
            gmail_message: Gmail message data
            extracted_metadata: Extracted metadata

        Returns:
            True if sensitive content detected
        """
        # Check subject
        subject = extracted_metadata.subject or ''

        # Check snippet (basic body content)
        snippet = gmail_message.get('snippet', '')

        # Combine text to check
        content_to_check = f"{subject} {snippet}".lower()

        # Check against sensitive patterns
        for pattern_name, pattern in self.sensitive_patterns.items():
            if pattern.search(content_to_check):
                logger.info(f"Sensitive content detected ({pattern_name}) in message {gmail_message.get('id', 'unknown')}")
                return True

        return False

    def _get_compiled_regex(self, pattern: str, case_sensitive: bool = False) -> Optional[re.Pattern]:
        """Get compiled regex pattern with caching.

        Args:
            pattern: Regex pattern string
            case_sensitive: Whether pattern is case sensitive

        Returns:
            Compiled regex pattern or None if invalid
        """
        cache_key = f"{pattern}_{case_sensitive}"

        if cache_key in self._regex_cache:
            return self._regex_cache[cache_key]

        try:
            flags = 0 if case_sensitive else re.IGNORECASE
            compiled_pattern = re.compile(pattern, flags)
            self._regex_cache[cache_key] = compiled_pattern
            return compiled_pattern
        except re.error as e:
            logger.warning(f"Invalid regex pattern '{pattern}': {e}")
            self._regex_cache[cache_key] = None
            return None

    def _get_action_priority(self, action: FilterAction) -> int:
        """Get priority level for filter action.

        Args:
            action: Filter action

        Returns:
            Priority level (higher number = higher priority)
        """
        priority_map = {
            FilterAction.LOG_ONLY: 1,
            FilterAction.MARK_SENSITIVE: 2,
            FilterAction.LIMITED_PROCESSING: 3,
            FilterAction.REQUIRE_REVIEW: 4,
            FilterAction.QUARANTINE: 5,
            FilterAction.EXCLUDE: 6
        }
        return priority_map.get(action, 1)

    async def _update_filter_stats(self, filter_obj: PrivacyFilter, processing_time_ms: int) -> None:
        """Update filter usage statistics.

        Args:
            filter_obj: Privacy filter
            processing_time_ms: Processing time in milliseconds
        """
        try:
            async with db_manager.get_session() as session:
                # Update match count and timing
                filter_obj.match_count += 1
                filter_obj.last_matched = datetime.utcnow()

                # Update average processing time
                total_time = filter_obj.total_processing_time_ms + processing_time_ms
                avg_time = total_time // filter_obj.match_count

                filter_obj.total_processing_time_ms = total_time
                filter_obj.avg_processing_time_ms = avg_time

                await session.commit()

        except Exception as e:
            logger.error(f"Error updating filter stats: {e}")

    async def _log_privacy_action(
        self,
        connection_id: int,
        gmail_message: Dict[str, Any],
        extracted_metadata: ExtractedMetadata,
        matched_filters: List[Dict[str, Any]],
        action_taken: FilterAction,
        processing_time_ms: int
    ) -> None:
        """Log privacy filter action for audit purposes.

        Args:
            connection_id: Gmail connection ID
            gmail_message: Gmail message data
            extracted_metadata: Extracted metadata
            matched_filters: List of matched filters
            action_taken: Final action taken
            processing_time_ms: Total processing time
        """
        try:
            async with db_manager.get_session() as session:
                # Create limited email metadata for audit (privacy-conscious)
                audit_metadata = {
                    'subject_hash': extracted_metadata.subject_hash,
                    'participant_count': extracted_metadata.participants.get('participant_count', 0),
                    'has_attachments': extracted_metadata.content_structure.has_attachments,
                    'message_format': extracted_metadata.content_structure.message_format,
                    'gmail_labels': extracted_metadata.gmail_labels[:5],  # Limit label list
                    'internal_date': extracted_metadata.internal_date.isoformat()
                }

                # Create audit log entries for each matched filter
                for filter_info in matched_filters:
                    audit_log = PrivacyAuditLog(
                        gmail_message_id=gmail_message['id'],
                        gmail_thread_id=gmail_message['threadId'],
                        connection_id=connection_id,
                        filter_id=filter_info['filter_id'],
                        filter_name=filter_info['filter_name'],
                        filter_type=filter_info['filter_type'],
                        action_taken=action_taken.value,
                        match_reason=filter_info['match_reason'],
                        matched_criteria=filter_info['matched_criteria'],
                        processing_time_ms=processing_time_ms,
                        email_metadata=audit_metadata,
                        user_context="system"
                    )
                    session.add(audit_log)

                await session.commit()
                logger.debug(f"Logged privacy action for message {gmail_message['id']}")

        except Exception as e:
            logger.error(f"Error logging privacy action: {e}")

    async def get_filter_statistics(self, connection_id: int, days: int = 30) -> Dict[str, Any]:
        """Get privacy filter statistics for a connection.

        Args:
            connection_id: Gmail connection ID
            days: Number of days to look back

        Returns:
            Filter statistics
        """
        async with db_manager.get_session() as session:
            since_date = datetime.utcnow() - timedelta(days=days)

            # Get filter match counts
            filter_stats = await session.execute(
                select(
                    PrivacyFilter.name,
                    PrivacyFilter.filter_type,
                    PrivacyFilter.match_count,
                    PrivacyFilter.avg_processing_time_ms
                )
                .where(PrivacyFilter.connection_id == connection_id)
                .order_by(desc(PrivacyFilter.match_count))
            )

            # Get recent audit logs
            audit_stats = await session.execute(
                select(
                    PrivacyAuditLog.action_taken,
                    func.count(PrivacyAuditLog.id).label('count')
                )
                .where(
                    and_(
                        PrivacyAuditLog.connection_id == connection_id,
                        PrivacyAuditLog.timestamp >= since_date
                    )
                )
                .group_by(PrivacyAuditLog.action_taken)
            )

            return {
                'filter_performance': [
                    {
                        'name': row[0],
                        'type': row[1],
                        'total_matches': row[2],
                        'avg_processing_time_ms': row[3]
                    }
                    for row in filter_stats
                ],
                'action_counts': {
                    row[0]: row[1] for row in audit_stats
                },
                'period_days': days
            }