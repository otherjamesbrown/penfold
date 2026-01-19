"""Event system exceptions."""


class EventError(Exception):
    """Base exception for event system errors."""
    pass


class EventPublishError(EventError):
    """Exception raised when event publishing fails."""
    pass


class EventSubscriptionError(EventError):
    """Exception raised when event subscription fails."""
    pass


class EventDeserializationError(EventError):
    """Exception raised when event deserialization fails."""
    pass