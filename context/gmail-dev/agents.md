# Gmail Integration Development Agent Context

This context enables AI agents to work effectively with Penfold's Gmail integration, implementing secure email ingestion, real-time synchronization, and multi-account management.

## Agent Expertise

**Primary Skills**: Gmail API, OAuth2 authentication, real-time sync, attachment processing, privacy controls

**Key Responsibilities**:
- Gmail account connection and OAuth2 flow management
- Historical email import and incremental sync
- Real-time notification handling (push/poll hybrid)
- Attachment extraction and content processing
- Multi-account coordination and scheduling
- Privacy filtering and compliance

## Key Components

| Component | Location | Purpose |
|-----------|----------|---------|
| OAuth2 Auth | `penf_lib/connectors/gmail/auth.py` | Secure authentication |
| Gmail Client | `penf_lib/connectors/gmail/client.py` | API wrapper |
| Sync Engine | `penf_lib/connectors/gmail/sync.py` | Email synchronization |
| Webhook Handler | `penf_lib/connectors/gmail/webhook.py` | Push notifications |
| Attachments | `penf_lib/connectors/gmail/attachments.py` | Content extraction |
| Multi-Account | `penf_lib/connectors/gmail/multi_account.py` | Account management |
| Privacy | `penf_lib/connectors/gmail/privacy.py` | Filter chain |
| CLI | `penf_lib/cli/gmail_commands.py` | User interface |

## Architectural Patterns (Production-Proven)

### OAuth2 Token Management

**Pattern**: Encrypted storage with automatic refresh

```python
from penf_lib.connectors.gmail.auth import GmailAuthManager

auth_manager = GmailAuthManager(encryption_key=config.encryption_key)

# Initiate OAuth flow
auth_url = await auth_manager.get_authorization_url(account_id)

# Complete flow with callback code
await auth_manager.complete_authorization(account_id, auth_code)

# Get valid token (auto-refreshes if needed)
token = await auth_manager.get_valid_token(account_id)
```

**Key Points**:
- Tokens encrypted with AES-256 at rest
- Refresh happens 5 minutes before expiration
- Revocation detected on API errors

### Real-Time Sync with Fallback

**Pattern**: Push notifications with polling fallback

```python
from penf_lib.connectors.gmail.sync import GmailSyncManager

sync_manager = GmailSyncManager(account_id)

# Start sync (tries push, falls back to poll)
await sync_manager.start()

# Handle incoming webhook
@app.post("/webhooks/gmail/{account_id}")
async def handle_webhook(account_id: str, request: Request):
    await sync_manager.handle_notification(await request.json())
```

**Key Points**:
- Push via Gmail Cloud Pub/Sub
- Poll interval: 60 seconds (fallback)
- Catch-up sync on reconnection

### Multi-Account Scheduling

**Pattern**: Priority-based concurrent sync

```python
from penf_lib.connectors.gmail.multi_account import AccountScheduler

scheduler = AccountScheduler(max_concurrent=3)

# Configure account priorities
accounts = [
    GmailAccount(email="work@company.com", priority=1),  # Highest
    GmailAccount(email="personal@gmail.com", priority=2),
    GmailAccount(email="newsletters@gmail.com", priority=3),  # Lowest
]

# Schedule sync (respects priorities and concurrency)
await scheduler.schedule_sync(accounts)
```

**Key Points**:
- Higher priority = more frequent sync
- Semaphore limits concurrent API calls
- Failures isolated per account

### Attachment Processing

**Pattern**: Background queue with format extractors

```python
from penf_lib.connectors.gmail.attachments import AttachmentProcessor

processor = AttachmentProcessor()

# Queue attachment for processing
task_id = await processor.queue_attachment(
    attachment_id=attachment.id,
    mime_type=attachment.mime_type,
    data=attachment.data
)

# Check processing status
status = await processor.get_status(task_id)
```

**Supported Formats**:
- PDF: Text extraction with PyPDF2
- DOCX: Content extraction with python-docx
- Images: OCR with Tesseract (optional)
- Plain text: Direct processing

### Privacy Filter Chain

**Pattern**: Configurable exclusion rules

```python
from penf_lib.connectors.gmail.privacy import PrivacyFilterChain, LabelFilter, SenderFilter

# Configure filters
filter_chain = PrivacyFilterChain()
filter_chain.add_filter(LabelFilter(["confidential", "personal"]))
filter_chain.add_filter(SenderFilter(excluded_domains=["private.com"]))

# Check email
should_process, reason = await filter_chain.should_process(email)
if not should_process:
    logger.info(f"Email filtered: {reason}")
```

## CLI Commands

```bash
# Account management
penf gmail connect              # Start OAuth flow
penf gmail list                 # List connected accounts
penf gmail disconnect <email>   # Remove account

# Sync operations
penf gmail sync --account=<email>           # Manual sync
penf gmail sync --all                       # Sync all accounts
penf gmail import --since="2024-01-01"      # Historical import

# Status and monitoring
penf gmail status               # Show sync status
penf gmail health               # Check connection health
penf gmail stats                # Show processing statistics

# Privacy controls
penf gmail filters list         # Show active filters
penf gmail filters add-label <label>        # Add label exclusion
penf gmail filters add-domain <domain>      # Add domain exclusion
```

## Configuration

```python
# config.py or environment variables
GMAIL_CONFIG = {
    # OAuth2
    "client_id": os.environ["GMAIL_CLIENT_ID"],
    "client_secret": os.environ["GMAIL_CLIENT_SECRET"],
    "redirect_uri": "http://localhost:8080/oauth/callback",

    # Sync settings
    "push_enabled": True,
    "poll_interval_seconds": 60,
    "max_concurrent_syncs": 3,

    # Import settings
    "batch_size": 100,
    "max_emails_per_import": 10000,

    # Attachment settings
    "max_attachment_size_mb": 25,
    "supported_mime_types": ["application/pdf", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"],

    # Rate limiting
    "quota_units_per_second": 250,
    "enable_rate_limiting": True,
}
```

## Error Handling

### Common Errors

| Error | Cause | Resolution |
|-------|-------|------------|
| `TokenExpiredError` | OAuth token expired | Auto-refresh triggers |
| `TokenRevokedError` | User revoked access | Prompt re-authorization |
| `RateLimitError` | API quota exceeded | Exponential backoff |
| `PushSetupError` | Webhook registration failed | Fall back to polling |
| `AttachmentError` | Extraction failed | Queue for retry |

### Retry Strategy

```python
# Exponential backoff for transient errors
RETRY_CONFIG = {
    "max_retries": 3,
    "base_delay_seconds": 1,
    "max_delay_seconds": 60,
    "exponential_base": 2,
}

# Rate limit handling
async def handle_rate_limit(response):
    retry_after = int(response.headers.get("Retry-After", 60))
    await asyncio.sleep(retry_after)
```

## Testing

```bash
# Unit tests
pytest tests/unit/test_gmail_auth.py
pytest tests/unit/test_gmail_sync.py
pytest tests/unit/test_privacy_filters.py

# Integration tests (requires test Gmail account)
GMAIL_TEST_ACCOUNT=test@gmail.com pytest tests/integration/test_gmail_api.py

# Performance tests
pytest tests/performance/test_gmail_performance.py --benchmark
```

### Test Fixtures

```python
@pytest.fixture
def mock_gmail_client():
    """Mock Gmail API client for unit tests"""
    return MockGmailClient(responses=SAMPLE_RESPONSES)

@pytest.fixture
def sample_email():
    """Sample email for testing"""
    return {
        "id": "msg_12345",
        "threadId": "thread_123",
        "labelIds": ["INBOX"],
        "snippet": "Test email content...",
        "payload": {...}
    }
```

## Integration Points

- **Event Framework (002)**: Publishes `content.ingested` events
- **AI Coordination (003)**: Emails feed entity extraction
- **Observability (011)**: Metrics and health monitoring
- **Database (001)**: Credential and sync state storage

## Performance Targets

| Metric | Target | Monitoring |
|--------|--------|------------|
| OAuth flow | <2 min | `gmail_oauth_duration_seconds` |
| Historical import | 100+ emails/min | `gmail_import_rate` |
| Real-time latency | <60 sec | `gmail_sync_latency_seconds` |
| Attachment success | >90% | `gmail_attachment_success_rate` |

## Related Documentation

- [Setup Guide](../../docs/gmail-integration/setup-guide.md)
- [API Reference](../../docs/gmail-integration/api-reference.md)
- [Architecture](../../docs/gmail-integration/architecture.md)
- [Troubleshooting](../../docs/gmail-integration/troubleshooting.md)
- [Architecture Patterns](../ARCHITECTURE.md) - Patterns 18-22
