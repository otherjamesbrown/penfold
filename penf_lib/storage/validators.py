"""Custom validation functions for Penfold database models.

This module provides comprehensive validation logic for database constraints
and field validation that can be reused across models and in application logic.
"""

import re
import hashlib
import uuid
from typing import Any, List, Optional, Union
from decimal import Decimal, InvalidOperation


# Validation constants
EMAIL_REGEX = re.compile(r'^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$')
CONTENT_HASH_REGEX = re.compile(r'^[a-fA-F0-9]{64}$')  # SHA-256 hex
UUID_REGEX = re.compile(r'^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$')
PHONE_REGEX = re.compile(r'^\+?1?[2-9]\d{2}[2-9]\d{2}\d{4}$')  # North American format

# Enumeration constants for database constraints
SOURCE_SYSTEMS = {'gmail', 'slack', 'zoom', 'calendar', 'drive', 'confluence', 'jira', 'teams'}
PROCESSING_STATUSES = {'pending', 'processing', 'completed', 'failed', 'retrying'}
ASSERTION_TYPES = {'decision', 'risk', 'commitment', 'milestone', 'outcome', 'dependency', 'assumption', 'issue'}
PROJECT_STATUSES = {'planning', 'active', 'on_hold', 'completed', 'cancelled', 'archived'}
PROJECT_PRIORITIES = {'critical', 'high', 'medium', 'low'}
TEAM_TYPES = {'department', 'project_team', 'working_group', 'committee'}
JOB_STATUSES = {'queued', 'in_progress', 'completed', 'failed', 'retrying', 'cancelled'}
VALIDATION_STATUSES = {'pending', 'approved', 'rejected', 'needs_review'}
ENTITY_TYPES = {'source', 'assertion', 'person', 'project', 'team'}
EVENT_TYPES = {'content.ingested', 'content.processed', 'assertion.extracted', 'person.resolved', 'project.updated'}
EVENT_PROCESSING_STATUSES = {'published', 'processing', 'completed', 'failed'}


class ValidationError(Exception):
    """Custom exception for validation errors."""
    pass


def validate_email(email: str) -> bool:
    """Validate email address format.

    Args:
        email: Email address to validate

    Returns:
        True if email is valid

    Raises:
        ValidationError: If email format is invalid
    """
    if not email or not isinstance(email, str):
        raise ValidationError("Email must be a non-empty string")

    email = email.strip().lower()
    if not EMAIL_REGEX.match(email):
        raise ValidationError(f"Invalid email format: {email}")

    if len(email) > 254:  # RFC 5321 limit
        raise ValidationError("Email address too long (max 254 characters)")

    return True


def validate_email_list(emails: List[str]) -> bool:
    """Validate a list of email addresses.

    Args:
        emails: List of email addresses to validate

    Returns:
        True if all emails are valid

    Raises:
        ValidationError: If any email format is invalid
    """
    if not emails:
        return True

    if not isinstance(emails, list):
        raise ValidationError("Emails must be a list")

    # Check for duplicates
    unique_emails = set()
    for email in emails:
        validate_email(email)
        normalized = email.strip().lower()
        if normalized in unique_emails:
            raise ValidationError(f"Duplicate email address: {normalized}")
        unique_emails.add(normalized)

    return True


def validate_content_hash(content_hash: str) -> bool:
    """Validate SHA-256 content hash format.

    Args:
        content_hash: Hash string to validate

    Returns:
        True if hash is valid

    Raises:
        ValidationError: If hash format is invalid
    """
    if not content_hash or not isinstance(content_hash, str):
        raise ValidationError("Content hash must be a non-empty string")

    if len(content_hash) != 64:
        raise ValidationError(f"Content hash must be 64 characters, got {len(content_hash)}")

    if not CONTENT_HASH_REGEX.match(content_hash):
        raise ValidationError("Content hash must be valid hexadecimal")

    return True


def validate_uuid(uuid_str: str) -> bool:
    """Validate UUID format.

    Args:
        uuid_str: UUID string to validate

    Returns:
        True if UUID is valid

    Raises:
        ValidationError: If UUID format is invalid
    """
    if not uuid_str:
        raise ValidationError("UUID must be provided")

    try:
        # Use uuid module for validation
        uuid_obj = uuid.UUID(str(uuid_str))
        return True
    except (ValueError, TypeError):
        raise ValidationError(f"Invalid UUID format: {uuid_str}")


def validate_confidence_score(score: Union[float, Decimal, None]) -> bool:
    """Validate confidence score is between 0.000 and 1.000.

    Args:
        score: Confidence score to validate

    Returns:
        True if score is valid

    Raises:
        ValidationError: If score is out of range
    """
    if score is None:
        return True

    try:
        decimal_score = Decimal(str(score))
    except (InvalidOperation, ValueError, TypeError):
        raise ValidationError(f"Confidence score must be a number, got {type(score)}")

    if decimal_score < 0 or decimal_score > 1:
        raise ValidationError(f"Confidence score must be between 0.000 and 1.000, got {decimal_score}")

    # Check precision (3 decimal places max)
    if decimal_score.as_tuple().exponent < -3:
        raise ValidationError("Confidence score precision limited to 3 decimal places")

    return True


def validate_processing_status(status: str) -> bool:
    """Validate processing status value.

    Args:
        status: Status value to validate

    Returns:
        True if status is valid

    Raises:
        ValidationError: If status is invalid
    """
    if not status or not isinstance(status, str):
        raise ValidationError("Processing status must be a non-empty string")

    if status not in PROCESSING_STATUSES:
        raise ValidationError(f"Invalid processing status: {status}. Must be one of: {sorted(PROCESSING_STATUSES)}")

    return True


def validate_source_system(system: str) -> bool:
    """Validate source system value.

    Args:
        system: Source system to validate

    Returns:
        True if system is valid

    Raises:
        ValidationError: If system is invalid
    """
    if not system or not isinstance(system, str):
        raise ValidationError("Source system must be a non-empty string")

    if system not in SOURCE_SYSTEMS:
        raise ValidationError(f"Invalid source system: {system}. Must be one of: {sorted(SOURCE_SYSTEMS)}")

    return True


def validate_assertion_type(assertion_type: str) -> bool:
    """Validate assertion type value.

    Args:
        assertion_type: Assertion type to validate

    Returns:
        True if type is valid

    Raises:
        ValidationError: If type is invalid
    """
    if not assertion_type or not isinstance(assertion_type, str):
        raise ValidationError("Assertion type must be a non-empty string")

    if assertion_type not in ASSERTION_TYPES:
        raise ValidationError(f"Invalid assertion type: {assertion_type}. Must be one of: {sorted(ASSERTION_TYPES)}")

    return True


def validate_project_status(status: str) -> bool:
    """Validate project status value.

    Args:
        status: Project status to validate

    Returns:
        True if status is valid

    Raises:
        ValidationError: If status is invalid
    """
    if not status or not isinstance(status, str):
        raise ValidationError("Project status must be a non-empty string")

    if status not in PROJECT_STATUSES:
        raise ValidationError(f"Invalid project status: {status}. Must be one of: {sorted(PROJECT_STATUSES)}")

    return True


def validate_project_priority(priority: str) -> bool:
    """Validate project priority value.

    Args:
        priority: Project priority to validate

    Returns:
        True if priority is valid

    Raises:
        ValidationError: If priority is invalid
    """
    if priority and priority not in PROJECT_PRIORITIES:
        raise ValidationError(f"Invalid project priority: {priority}. Must be one of: {sorted(PROJECT_PRIORITIES)}")

    return True


def validate_team_type(team_type: str) -> bool:
    """Validate team type value.

    Args:
        team_type: Team type to validate

    Returns:
        True if type is valid

    Raises:
        ValidationError: If type is invalid
    """
    if team_type and team_type not in TEAM_TYPES:
        raise ValidationError(f"Invalid team type: {team_type}. Must be one of: {sorted(TEAM_TYPES)}")

    return True


def validate_phone_number(phone: str) -> bool:
    """Validate phone number format (North American).

    Args:
        phone: Phone number to validate

    Returns:
        True if phone is valid

    Raises:
        ValidationError: If phone format is invalid
    """
    if not phone or not isinstance(phone, str):
        raise ValidationError("Phone number must be a non-empty string")

    # Remove common formatting characters
    cleaned = re.sub(r'[-()\s]', '', phone.strip())

    if not PHONE_REGEX.match(cleaned):
        raise ValidationError(f"Invalid phone number format: {phone}")

    return True


def validate_phone_list(phones: List[str]) -> bool:
    """Validate a list of phone numbers.

    Args:
        phones: List of phone numbers to validate

    Returns:
        True if all phones are valid

    Raises:
        ValidationError: If any phone format is invalid
    """
    if not phones:
        return True

    if not isinstance(phones, list):
        raise ValidationError("Phone numbers must be a list")

    for phone in phones:
        validate_phone_number(phone)

    return True


def validate_entity_type(entity_type: str) -> bool:
    """Validate entity type for embeddings.

    Args:
        entity_type: Entity type to validate

    Returns:
        True if type is valid

    Raises:
        ValidationError: If type is invalid
    """
    if not entity_type or not isinstance(entity_type, str):
        raise ValidationError("Entity type must be a non-empty string")

    if entity_type not in ENTITY_TYPES:
        raise ValidationError(f"Invalid entity type: {entity_type}. Must be one of: {sorted(ENTITY_TYPES)}")

    return True


def validate_array_not_empty(array: List[Any], field_name: str) -> bool:
    """Validate that an array field is not empty when provided.

    Args:
        array: Array to validate
        field_name: Name of field for error messages

    Returns:
        True if array is valid

    Raises:
        ValidationError: If array is empty when provided
    """
    if array is not None and isinstance(array, list) and len(array) == 0:
        raise ValidationError(f"{field_name} cannot be an empty array")

    return True


def validate_string_length(value: str, field_name: str, min_length: int = 1, max_length: int = None) -> bool:
    """Validate string length constraints.

    Args:
        value: String to validate
        field_name: Name of field for error messages
        min_length: Minimum allowed length
        max_length: Maximum allowed length

    Returns:
        True if string length is valid

    Raises:
        ValidationError: If string length is invalid
    """
    if value is None:
        return True

    if not isinstance(value, str):
        raise ValidationError(f"{field_name} must be a string")

    if len(value) < min_length:
        raise ValidationError(f"{field_name} must be at least {min_length} characters, got {len(value)}")

    if max_length and len(value) > max_length:
        raise ValidationError(f"{field_name} must be at most {max_length} characters, got {len(value)}")

    return True


def validate_positive_integer(value: int, field_name: str) -> bool:
    """Validate positive integer value.

    Args:
        value: Integer to validate
        field_name: Name of field for error messages

    Returns:
        True if integer is positive

    Raises:
        ValidationError: If integer is not positive
    """
    if value is None:
        return True

    if not isinstance(value, int):
        raise ValidationError(f"{field_name} must be an integer")

    if value <= 0:
        raise ValidationError(f"{field_name} must be positive, got {value}")

    return True


def validate_non_negative_integer(value: int, field_name: str) -> bool:
    """Validate non-negative integer value.

    Args:
        value: Integer to validate
        field_name: Name of field for error messages

    Returns:
        True if integer is non-negative

    Raises:
        ValidationError: If integer is negative
    """
    if value is None:
        return True

    if not isinstance(value, int):
        raise ValidationError(f"{field_name} must be an integer")

    if value < 0:
        raise ValidationError(f"{field_name} must be non-negative, got {value}")

    return True


def generate_content_hash(content: str) -> str:
    """Generate SHA-256 hash for content.

    Args:
        content: Content to hash

    Returns:
        64-character hexadecimal hash
    """
    if not content:
        return ""

    return hashlib.sha256(content.encode('utf-8')).hexdigest()


def validate_job_status(status: str) -> bool:
    """Validate job status value.

    Args:
        status: Job status to validate

    Returns:
        True if status is valid

    Raises:
        ValidationError: If status is invalid
    """
    if not status or not isinstance(status, str):
        raise ValidationError("Job status must be a non-empty string")

    if status not in JOB_STATUSES:
        raise ValidationError(f"Invalid job status: {status}. Must be one of: {sorted(JOB_STATUSES)}")

    return True


def validate_validation_status(status: str) -> bool:
    """Validate validation status value.

    Args:
        status: Validation status to validate

    Returns:
        True if status is valid

    Raises:
        ValidationError: If status is invalid
    """
    if status and status not in VALIDATION_STATUSES:
        raise ValidationError(f"Invalid validation status: {status}. Must be one of: {sorted(VALIDATION_STATUSES)}")

    return True


def validate_event_type(event_type: str) -> bool:
    """Validate event type value.

    Args:
        event_type: Event type to validate

    Returns:
        True if type is valid

    Raises:
        ValidationError: If type is invalid
    """
    if not event_type or not isinstance(event_type, str):
        raise ValidationError("Event type must be a non-empty string")

    # Allow flexible event types but require dotted notation
    if '.' not in event_type:
        raise ValidationError("Event type must use dotted notation (e.g., 'content.ingested')")

    if len(event_type) > 100:
        raise ValidationError("Event type must be 100 characters or less")

    return True


def validate_event_processing_status(status: str) -> bool:
    """Validate event processing status value.

    Args:
        status: Event processing status to validate

    Returns:
        True if status is valid

    Raises:
        ValidationError: If status is invalid
    """
    if not status or not isinstance(status, str):
        raise ValidationError("Event processing status must be a non-empty string")

    if status not in EVENT_PROCESSING_STATUSES:
        raise ValidationError(f"Invalid event processing status: {status}. Must be one of: {sorted(EVENT_PROCESSING_STATUSES)}")

    return True


def validate_priority(priority: int, min_priority: int = 1, max_priority: int = 1000) -> bool:
    """Validate priority value.

    Args:
        priority: Priority value to validate
        min_priority: Minimum allowed priority
        max_priority: Maximum allowed priority

    Returns:
        True if priority is valid

    Raises:
        ValidationError: If priority is out of range
    """
    if priority is None:
        return True

    if not isinstance(priority, int):
        raise ValidationError("Priority must be an integer")

    if priority < min_priority or priority > max_priority:
        raise ValidationError(f"Priority must be between {min_priority} and {max_priority}, got {priority}")

    return True


def validate_retry_count(retry_count: int, max_retries: int = None) -> bool:
    """Validate retry count against max retries.

    Args:
        retry_count: Current retry count
        max_retries: Maximum allowed retries

    Returns:
        True if retry count is valid

    Raises:
        ValidationError: If retry count exceeds maximum
    """
    if retry_count is None:
        return True

    if not isinstance(retry_count, int):
        raise ValidationError("Retry count must be an integer")

    if retry_count < 0:
        raise ValidationError("Retry count must be non-negative")

    if max_retries is not None and retry_count > max_retries:
        raise ValidationError(f"Retry count {retry_count} exceeds maximum retries {max_retries}")

    return True


def validate_vector_dimensions(embedding: List[float], expected_dimensions: int = 768) -> bool:
    """Validate vector embedding dimensions.

    Args:
        embedding: Vector embedding to validate
        expected_dimensions: Expected number of dimensions

    Returns:
        True if dimensions are correct

    Raises:
        ValidationError: If dimensions are incorrect
    """
    if embedding is None:
        raise ValidationError("Embedding cannot be None")

    if not isinstance(embedding, (list, tuple)):
        raise ValidationError("Embedding must be a list or tuple of numbers")

    if len(embedding) != expected_dimensions:
        raise ValidationError(f"Embedding must have {expected_dimensions} dimensions, got {len(embedding)}")

    # Validate all elements are numbers
    for i, value in enumerate(embedding):
        if not isinstance(value, (int, float)):
            raise ValidationError(f"Embedding value at index {i} must be a number, got {type(value)}")

        # Check for invalid float values
        if isinstance(value, float) and (value != value or value == float('inf') or value == float('-inf')):
            raise ValidationError(f"Embedding value at index {i} is not a valid number: {value}")

    return True