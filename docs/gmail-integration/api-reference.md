# Gmail Integration API Reference

This document provides comprehensive API reference for the Gmail integration components, including all public interfaces, data models, and usage examples.

## Table of Contents

- [Authentication API](#authentication-api)
- [Gmail Client API](#gmail-client-api)
- [Sync Coordinator API](#sync-coordinator-api)
- [Real-time Monitor API](#real-time-monitor-api)
- [Privacy Filter API](#privacy-filter-api)
- [Attachment Processor API](#attachment-processor-api)
- [Data Models](#data-models)
- [Event Schemas](#event-schemas)
- [Configuration API](#configuration-api)
- [CLI Commands](#cli-commands)

## Authentication API

### OAuth2Manager

Handles Gmail OAuth2 authentication and credential management.

```python
from penf_lib.connectors.gmail.auth import OAuth2Manager

class OAuth2Manager:
    """Secure OAuth2 credential management for Gmail API access."""
```

#### Methods

##### `start_oauth_flow(account_id: str) -> AuthorizationURL`

Initiates OAuth2 authorization flow for a Gmail account.

**Parameters:**
- `account_id` (str): Unique identifier for the Gmail account

**Returns:**
- `AuthorizationURL`: Object containing the authorization URL and flow state

**Example:**
```python
auth_manager = OAuth2Manager(encryption_key="your-key")
auth_url = await auth_manager.start_oauth_flow("user@gmail.com")
print(f"Visit: {auth_url.url}")
```

**Raises:**
- `AuthenticationError`: If OAuth2 configuration is invalid
- `DuplicateAccountError`: If account is already authenticated

##### `complete_oauth_flow(account_id: str, auth_code: str) -> bool`

Completes OAuth2 authorization flow and stores encrypted credentials.

**Parameters:**
- `account_id` (str): Gmail account identifier
- `auth_code` (str): Authorization code from OAuth2 callback

**Returns:**
- `bool`: True if authentication successful

**Example:**
```python
success = await auth_manager.complete_oauth_flow(
    account_id="user@gmail.com",
    auth_code="4/0AX4XfWh..."
)
```

**Raises:**
- `AuthenticationError`: If authorization code is invalid or expired
- `EncryptionError`: If credential encryption fails

##### `refresh_token(account_id: str) -> Optional[AccessToken]`

Refreshes OAuth2 access token for continued API access.

**Parameters:**
- `account_id` (str): Gmail account identifier

**Returns:**
- `Optional[AccessToken]`: New access token, or None if refresh failed

**Example:**
```python
token = await auth_manager.refresh_token("user@gmail.com")
if token is None:
    # Re-authentication required
    await auth_manager.start_oauth_flow("user@gmail.com")
```

##### `revoke_access(account_id: str) -> bool`

Revokes OAuth2 access and removes stored credentials.

**Parameters:**
- `account_id` (str): Gmail account identifier

**Returns:**
- `bool`: True if revocation successful

**Example:**
```python
await auth_manager.revoke_access("user@gmail.com")
```

##### `get_accounts() -> List[str]`

Returns list of authenticated Gmail accounts.

**Returns:**
- `List[str]`: List of account identifiers

**Example:**
```python
accounts = await auth_manager.get_accounts()
print(f"Authenticated accounts: {accounts}")
```

## Gmail Client API

### GmailAPIClient

High-level Gmail API client with rate limiting and error handling.

```python
from penf_lib.connectors.gmail.client import GmailAPIClient

class GmailAPIClient:
    """High-level Gmail API client with rate limiting and error handling."""
```

#### Methods

##### `list_messages(account_id: str, query: Optional[str] = None, max_results: int = 100) -> List[MessageMetadata]`

Lists Gmail messages matching specified criteria.

**Parameters:**
- `account_id` (str): Gmail account identifier
- `query` (Optional[str]): Gmail search query (e.g., "from:example.com")
- `max_results` (int): Maximum number of results to return

**Returns:**
- `List[MessageMetadata]`: List of message metadata objects

**Example:**
```python
client = GmailAPIClient(auth_manager)

# List recent messages
messages = await client.list_messages("user@gmail.com", max_results=50)

# Search messages
project_emails = await client.list_messages(
    account_id="user@gmail.com",
    query="subject:Atlas project",
    max_results=100
)
```

##### `get_message(account_id: str, message_id: str, include_attachments: bool = True) -> EmailMessage`

Retrieves full Gmail message content including attachments.

**Parameters:**
- `account_id` (str): Gmail account identifier
- `message_id` (str): Gmail message ID
- `include_attachments` (bool): Whether to download attachments

**Returns:**
- `EmailMessage`: Complete message object with content and metadata

**Example:**
```python
message = await client.get_message(
    account_id="user@gmail.com",
    message_id="1234567890abcdef",
    include_attachments=True
)

print(f"Subject: {message.subject}")
print(f"Sender: {message.sender.email}")
print(f"Attachments: {len(message.attachments)}")
```

##### `get_thread(account_id: str, thread_id: str) -> EmailThread`

Retrieves complete Gmail thread with all messages.

**Parameters:**
- `account_id` (str): Gmail account identifier
- `thread_id` (str): Gmail thread ID

**Returns:**
- `EmailThread`: Thread object containing all messages

**Example:**
```python
thread = await client.get_thread("user@gmail.com", "thread_abc123")
print(f"Thread has {len(thread.messages)} messages")
```

##### `get_labels(account_id: str) -> List[GmailLabel]`

Retrieves Gmail labels for an account.

**Parameters:**
- `account_id` (str): Gmail account identifier

**Returns:**
- `List[GmailLabel]`: List of Gmail labels

**Example:**
```python
labels = await client.get_labels("user@gmail.com")
for label in labels:
    print(f"Label: {label.name} ({label.message_count} messages)")
```

## Sync Coordinator API

### SyncCoordinator

Orchestrates email synchronization across multiple accounts.

```python
from penf_lib.connectors.gmail.sync import SyncCoordinator

class SyncCoordinator:
    """Orchestrates Gmail synchronization across multiple accounts."""
```

#### Methods

##### `start_historical_import(account_id: str, date_range: DateRange, filters: PrivacyFilters) -> SyncOperation`

Starts historical email import with progress tracking.

**Parameters:**
- `account_id` (str): Gmail account identifier
- `date_range` (DateRange): Date range for import
- `filters` (PrivacyFilters): Privacy filtering configuration

**Returns:**
- `SyncOperation`: Operation object for tracking progress

**Example:**
```python
sync = SyncCoordinator(client, event_publisher)

operation = await sync.start_historical_import(
    account_id="user@gmail.com",
    date_range=DateRange(days_back=30),
    filters=PrivacyFilters(exclude_labels=["Personal"])
)

# Monitor progress
while not operation.is_complete():
    status = await operation.get_status()
    print(f"Progress: {status.progress.percent:.1f}%")
    await asyncio.sleep(5)
```

##### `incremental_sync(account_id: str) -> SyncResult`

Performs incremental sync for new or modified messages.

**Parameters:**
- `account_id` (str): Gmail account identifier

**Returns:**
- `SyncResult`: Result object with sync statistics

**Example:**
```python
result = await sync.incremental_sync("user@gmail.com")
print(f"Processed {result.new_messages} new messages")
print(f"Updated {result.updated_messages} existing messages")
```

##### `get_sync_status(account_id: str) -> SyncStatus`

Gets current sync status and metrics for an account.

**Parameters:**
- `account_id` (str): Gmail account identifier

**Returns:**
- `SyncStatus`: Current sync status and statistics

**Example:**
```python
status = await sync.get_sync_status("user@gmail.com")
print(f"Last sync: {status.last_sync_at}")
print(f"Total messages: {status.total_messages_processed}")
```

## Real-time Monitor API

### RealtimeMonitor

Handles real-time Gmail change notifications.

```python
from penf_lib.connectors.gmail.webhook import RealtimeMonitor

class RealtimeMonitor:
    """Handles real-time Gmail change notifications."""
```

#### Methods

##### `setup_push_notifications(account_id: str) -> bool`

Configures Gmail Push notifications for real-time sync.

**Parameters:**
- `account_id` (str): Gmail account identifier

**Returns:**
- `bool`: True if setup successful

**Example:**
```python
monitor = RealtimeMonitor(sync_coordinator)
success = await monitor.setup_push_notifications("user@gmail.com")
if success:
    print("Real-time monitoring enabled")
```

##### `start_monitoring() -> None`

Starts the real-time monitoring service.

**Example:**
```python
await monitor.start_monitoring()
# Service runs until stopped
```

##### `stop_monitoring() -> None`

Stops the real-time monitoring service.

**Example:**
```python
await monitor.stop_monitoring()
```

## Privacy Filter API

### PrivacyFilterEngine

Implements configurable privacy controls for email processing.

```python
from penf_lib.connectors.gmail.privacy import PrivacyFilterEngine

class PrivacyFilterEngine:
    """Configurable privacy filtering for email content."""
```

#### Methods

##### `should_process_email(email: EmailMessage, account_id: str) -> FilterDecision`

Determines if email should be processed based on privacy rules.

**Parameters:**
- `email` (EmailMessage): Email to evaluate
- `account_id` (str): Gmail account identifier

**Returns:**
- `FilterDecision`: Decision with reasoning

**Example:**
```python
filter_engine = PrivacyFilterEngine(privacy_config)
decision = await filter_engine.should_process_email(email, "user@gmail.com")

if decision.should_exclude:
    print(f"Email excluded: {decision.reason}")
else:
    print("Email approved for processing")
```

##### `filter_email_content(email: EmailMessage) -> EmailMessage`

Applies content filtering to email body and metadata.

**Parameters:**
- `email` (EmailMessage): Original email message

**Returns:**
- `EmailMessage`: Filtered email message

**Example:**
```python
filtered_email = await filter_engine.filter_email_content(email)
# Sensitive content has been redacted or removed
```

## Attachment Processor API

### AttachmentProcessor

Handles email attachment processing and content extraction.

```python
from penf_lib.connectors.gmail.attachments import AttachmentProcessor

class AttachmentProcessor:
    """Handles email attachment processing and content extraction."""
```

#### Methods

##### `process_attachments(email: EmailMessage) -> List[ProcessedAttachment]`

Processes all attachments in an email message.

**Parameters:**
- `email` (EmailMessage): Email containing attachments

**Returns:**
- `List[ProcessedAttachment]`: List of processed attachments

**Example:**
```python
processor = AttachmentProcessor(storage, queue)
attachments = await processor.process_attachments(email)

for attachment in attachments:
    print(f"Processed: {attachment.filename}")
    if attachment.content_extracted:
        print(f"Content: {attachment.extracted_text[:100]}...")
```

##### `extract_content(attachment: Attachment) -> Optional[str]`

Extracts text content from a single attachment.

**Parameters:**
- `attachment` (Attachment): Attachment to process

**Returns:**
- `Optional[str]`: Extracted text content, or None if extraction failed

**Example:**
```python
content = await processor.extract_content(attachment)
if content:
    print(f"Extracted {len(content)} characters")
```

## Data Models

### Core Models

#### EmailMessage

```python
@dataclass
class EmailMessage:
    """Represents a complete Gmail message with metadata and content."""

    # Message identifiers
    id: str                           # Gmail message ID
    thread_id: str                    # Gmail thread ID
    account_id: str                   # Associated Gmail account

    # Message metadata
    subject: str
    sender: EmailParticipant
    recipients: List[EmailParticipant]
    cc_recipients: List[EmailParticipant] = field(default_factory=list)
    bcc_recipients: List[EmailParticipant] = field(default_factory=list)
    timestamp: datetime
    labels: List[str] = field(default_factory=list)

    # Message content
    body_text: str
    body_html: Optional[str] = None
    attachments: List[Attachment] = field(default_factory=list)

    # Processing metadata
    is_read: bool = False
    is_starred: bool = False
    priority: str = "normal"
    privacy_filtered: bool = False
    processing_status: str = "pending"
```

#### EmailParticipant

```python
@dataclass
class EmailParticipant:
    """Represents an email participant (sender or recipient)."""

    email: str
    name: Optional[str] = None
    display_name: Optional[str] = None

    @property
    def formatted_address(self) -> str:
        """Returns formatted email address with name."""
        if self.name:
            return f"{self.name} <{self.email}>"
        return self.email
```

#### Attachment

```python
@dataclass
class Attachment:
    """Represents an email attachment."""

    # Attachment metadata
    id: str                           # Gmail attachment ID
    filename: str
    mime_type: str
    size_bytes: int

    # Content and processing
    content: Optional[bytes] = None
    storage_path: Optional[str] = None
    extracted_text: Optional[str] = None
    processing_status: str = "pending"
    processing_error: Optional[str] = None

    @property
    def is_text_extractable(self) -> bool:
        """Returns True if text extraction is supported."""
        extractable_types = {
            "application/pdf",
            "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
            "text/plain",
            "image/jpeg",
            "image/png"
        }
        return self.mime_type in extractable_types
```

#### EmailThread

```python
@dataclass
class EmailThread:
    """Represents a Gmail conversation thread."""

    # Thread metadata
    id: str                           # Gmail thread ID
    account_id: str                   # Associated Gmail account
    subject: str                      # Thread subject
    participants: List[EmailParticipant]
    labels: List[str] = field(default_factory=list)

    # Thread content
    messages: List[EmailMessage] = field(default_factory=list)
    message_count: int = 0

    # Thread timeline
    first_message_at: Optional[datetime] = None
    last_message_at: Optional[datetime] = None
    created_at: datetime = field(default_factory=datetime.now)
    updated_at: datetime = field(default_factory=datetime.now)

    def get_chronological_messages(self) -> List[EmailMessage]:
        """Returns messages sorted chronologically."""
        return sorted(self.messages, key=lambda m: m.timestamp)
```

### Configuration Models

#### PrivacyConfig

```python
@dataclass
class PrivacyConfig:
    """Privacy filtering configuration for Gmail integration."""

    # Label-based filtering
    exclude_labels: List[str] = field(default_factory=list)
    include_only_labels: List[str] = field(default_factory=list)

    # Content-based filtering
    exclude_patterns: List[str] = field(default_factory=list)
    content_redaction: Dict[str, str] = field(default_factory=dict)

    # Sender/domain filtering
    exclude_domains: List[str] = field(default_factory=list)
    exclude_senders: List[str] = field(default_factory=list)
    trusted_domains: List[str] = field(default_factory=list)

    # Audit and compliance
    audit_enabled: bool = True
    retention_days: Optional[int] = None

    def is_exclude_mode(self) -> bool:
        """Returns True if operating in exclude mode (default)."""
        return len(self.include_only_labels) == 0

    def is_allowlist_mode(self) -> bool:
        """Returns True if operating in allowlist mode."""
        return len(self.include_only_labels) > 0
```

#### GmailConfig

```python
@dataclass
class GmailConfig:
    """Gmail integration configuration."""

    # Authentication
    credentials_file: str
    encryption_key: str

    # Sync configuration
    realtime_sync: bool = True
    sync_interval: int = 60           # seconds
    batch_size: int = 100
    import_days_back: int = 90

    # Performance settings
    max_concurrent_requests: int = 5
    rate_limit_requests_per_second: int = 250
    retry_attempts: int = 3
    request_timeout: int = 30

    # Privacy and filtering
    privacy_filters: PrivacyConfig = field(default_factory=PrivacyConfig)

    # Attachment processing
    attachments_enabled: bool = True
    max_attachment_size_mb: int = 10
    extract_content: bool = True
    supported_formats: List[str] = field(
        default_factory=lambda: ["pdf", "docx", "txt", "jpeg", "png"]
    )
```

## Event Schemas

### GmailContentEvent

```python
@dataclass
class GmailContentEvent:
    """Event schema for Gmail content ingestion."""

    # Standard event fields
    event_type: str = "content.ingested"
    source_type: str = "gmail"
    source_id: str                    # Gmail message ID
    account_id: str                   # Gmail account identifier
    timestamp: datetime = field(default_factory=datetime.now)
    event_id: str = field(default_factory=lambda: str(uuid.uuid4()))

    # Gmail-specific metadata
    message_id: str
    thread_id: str
    subject: str
    sender: EmailParticipant
    recipients: List[EmailParticipant]
    message_timestamp: datetime
    labels: List[str]

    # Content
    body_text: str
    body_html: Optional[str] = None
    attachments: List[AttachmentReference] = field(default_factory=list)

    # Processing context
    privacy_filtered: bool = False
    thread_context: Optional[ThreadContext] = None
    extraction_metadata: Dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> dict:
        """Serialize event to dictionary for publishing."""
        return asdict(self)

    @classmethod
    def from_dict(cls, data: dict) -> "GmailContentEvent":
        """Deserialize event from dictionary."""
        return cls(**data)
```

## Configuration API

### GmailConfigManager

```python
from penf_lib.connectors.gmail.config import GmailConfigManager

class GmailConfigManager:
    """Manages Gmail integration configuration."""

    def load_config(self, config_path: str) -> GmailConfig:
        """Load configuration from file."""

    def save_config(self, config: GmailConfig, config_path: str) -> None:
        """Save configuration to file."""

    def update_privacy_filters(
        self,
        account_id: str,
        filters: PrivacyConfig
    ) -> None:
        """Update privacy filters for specific account."""

    def get_account_config(self, account_id: str) -> AccountConfig:
        """Get account-specific configuration."""
```

## CLI Commands

### Gmail Integration Commands

#### `penf gmail connect`

Connect a Gmail account using OAuth2 authentication.

```bash
# Connect primary account
penf gmail connect

# Connect specific account
penf gmail connect --account work@company.com

# Connect with custom scopes
penf gmail connect --scopes readonly,labels
```

#### `penf gmail import`

Import historical emails from connected accounts.

```bash
# Import with default settings
penf gmail import

# Import specific date range
penf gmail import --from-date 2025-01-01 --to-date 2025-12-31

# Import with privacy filters
penf gmail import --exclude-labels "Personal,Banking"

# Dry run to preview import
penf gmail import --dry-run
```

#### `penf gmail sync`

Perform manual synchronization of Gmail accounts.

```bash
# Sync all accounts
penf gmail sync

# Sync specific account
penf gmail sync --account user@gmail.com

# Force full sync (ignore incremental state)
penf gmail sync --full
```

#### `penf gmail status`

Display Gmail integration status and statistics.

```bash
# Show status for all accounts
penf gmail status

# Show detailed status for specific account
penf gmail status --account user@gmail.com --verbose

# Show sync history
penf gmail status --history
```

#### `penf gmail config`

Manage Gmail integration configuration.

```bash
# List current configuration
penf gmail config --list

# Update sync interval
penf gmail config --sync-interval 300

# Update privacy filters
penf gmail config --exclude-labels "Spam,Trash,Personal"

# Enable/disable attachments
penf gmail config --attachments-enabled true
```

## Error Handling

### Exception Hierarchy

```python
class GmailIntegrationError(Exception):
    """Base exception for Gmail integration errors."""

class AuthenticationError(GmailIntegrationError):
    """OAuth2 authentication failures."""

class APIQuotaExceededError(GmailIntegrationError):
    """Gmail API quota limit exceeded."""

class RateLimitError(GmailIntegrationError):
    """Rate limiting temporarily blocking requests."""

class PrivacyViolationError(GmailIntegrationError):
    """Content violates privacy filtering rules."""

class AttachmentProcessingError(GmailIntegrationError):
    """Attachment download or extraction failed."""

class SyncOperationError(GmailIntegrationError):
    """Email synchronization operation failed."""
```

### Error Response Format

```python
@dataclass
class ErrorResponse:
    """Standardized error response format."""

    error_type: str
    error_code: str
    message: str
    details: Dict[str, Any] = field(default_factory=dict)
    timestamp: datetime = field(default_factory=datetime.now)
    retry_after: Optional[int] = None  # seconds

    def is_retryable(self) -> bool:
        """Returns True if error condition is temporary."""
        retryable_codes = {
            "rate_limited",
            "temporary_failure",
            "network_error",
            "service_unavailable"
        }
        return self.error_code in retryable_codes
```

## Usage Examples

### Complete Integration Example

```python
import asyncio
from penf_lib.connectors.gmail import (
    OAuth2Manager,
    GmailAPIClient,
    SyncCoordinator,
    RealtimeMonitor,
    PrivacyFilterEngine,
    AttachmentProcessor
)

async def setup_gmail_integration():
    """Complete Gmail integration setup example."""

    # Initialize authentication
    auth_manager = OAuth2Manager(encryption_key="your-encryption-key")

    # Connect Gmail account
    auth_url = await auth_manager.start_oauth_flow("user@gmail.com")
    print(f"Visit: {auth_url.url}")

    # Simulate OAuth2 callback (in real app, this comes from web server)
    auth_code = input("Enter authorization code: ")
    success = await auth_manager.complete_oauth_flow("user@gmail.com", auth_code)

    if not success:
        raise Exception("Authentication failed")

    # Initialize Gmail client
    client = GmailAPIClient(auth_manager)

    # Setup privacy filtering
    privacy_config = PrivacyConfig(
        exclude_labels=["Personal", "Spam"],
        exclude_patterns=[r"\b\d{3}-\d{2}-\d{4}\b"],  # SSN pattern
        audit_enabled=True
    )
    privacy_engine = PrivacyFilterEngine(privacy_config)

    # Setup attachment processing
    attachment_processor = AttachmentProcessor(
        storage=FileStorage("/tmp/attachments"),
        queue=BackgroundQueue()
    )

    # Initialize sync coordinator
    sync_coordinator = SyncCoordinator(
        client=client,
        privacy_engine=privacy_engine,
        attachment_processor=attachment_processor,
        event_publisher=event_publisher
    )

    # Start historical import
    import_operation = await sync_coordinator.start_historical_import(
        account_id="user@gmail.com",
        date_range=DateRange(days_back=30),
        filters=privacy_config
    )

    # Monitor import progress
    while not import_operation.is_complete():
        status = await import_operation.get_status()
        print(f"Import progress: {status.progress.percent:.1f}%")
        await asyncio.sleep(5)

    print("Historical import completed")

    # Setup real-time monitoring
    monitor = RealtimeMonitor(sync_coordinator)
    await monitor.setup_push_notifications("user@gmail.com")
    await monitor.start_monitoring()

    print("Gmail integration fully configured and running")

# Run the setup
if __name__ == "__main__":
    asyncio.run(setup_gmail_integration())
```

This API reference provides comprehensive documentation for all Gmail integration components. Each API includes detailed method signatures, parameters, return values, usage examples, and error handling information to enable effective development and integration.