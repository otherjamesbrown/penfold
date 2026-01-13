# Gmail Integration Research Findings

**Phase**: Phase 0 Research
**Date**: 2026-01-13
**Focus**: Technical implementation patterns for Gmail API integration with OAuth2 security, real-time sync, and attachment processing

## 1. Gmail API Best Practices and Authentication Patterns

### OAuth2 Flow Implementation

**Recommended Approach**: Server-side OAuth2 flow with refresh token storage
```python
# Best practice pattern for Gmail OAuth2
from google.auth.transport.requests import Request
from google.oauth2.credentials import Credentials
from google_auth_oauthlib.flow import Flow

class GmailAuthenticator:
    def __init__(self, client_config_path, scopes):
        self.scopes = scopes
        self.flow = Flow.from_client_secrets_file(
            client_config_path,
            scopes=self.scopes,
            redirect_uri='http://localhost:8080/callback'
        )

    async def get_authorization_url(self):
        auth_url, _ = self.flow.authorization_url(prompt='consent')
        return auth_url

    async def handle_callback(self, authorization_response):
        self.flow.fetch_token(authorization_response=authorization_response)
        return self.flow.credentials
```

**Key Security Considerations**:
- Use PKCE (Proof Key for Code Exchange) for additional security
- Store refresh tokens encrypted at rest (AES-256)
- Implement token rotation on refresh
- Set appropriate token expiration handling

**Scopes Required**:
- `https://www.googleapis.com/auth/gmail.readonly` - Read access to Gmail
- `https://www.googleapis.com/auth/gmail.modify` - For webhook setup (push notifications)

### Rate Limiting and Quota Management

**Gmail API Quotas** (2026 current limits):
- **Per-user rate limit**: 250 quota units/second
- **Daily quota**: 1 billion quota units/day
- **Batch requests**: Up to 100 operations per batch

**Quota Unit Costs**:
- List messages: 5 units per request
- Get message: 5 units per request
- Get attachment: 5 units per request
- Watch (setup push): 10 units per request

**Implementation Strategy**:
```python
import asyncio
from asyncio import Semaphore

class RateLimitedGmailClient:
    def __init__(self, max_concurrent=10, requests_per_second=20):
        self.semaphore = Semaphore(max_concurrent)
        self.rate_limiter = AsyncRateLimiter(requests_per_second)

    async def make_request(self, request_func, *args, **kwargs):
        async with self.semaphore:
            await self.rate_limiter.acquire()
            return await request_func(*args, **kwargs)
```

## 2. OAuth2 Security Implementation for Long-Lived Applications

### Credential Storage Security

**AES-256 Encryption Pattern**:
```python
from cryptography.fernet import Fernet
import os
import base64

class CredentialEncryption:
    def __init__(self, key_path=None):
        if key_path and os.path.exists(key_path):
            with open(key_path, 'rb') as f:
                self.key = f.read()
        else:
            self.key = Fernet.generate_key()
            if key_path:
                with open(key_path, 'wb') as f:
                    f.write(self.key)
        self.cipher = Fernet(self.key)

    def encrypt_credentials(self, credentials_dict):
        json_creds = json.dumps(credentials_dict)
        return self.cipher.encrypt(json_creds.encode())

    def decrypt_credentials(self, encrypted_data):
        decrypted = self.cipher.decrypt(encrypted_data)
        return json.loads(decrypted.decode())
```

### Token Refresh Strategy

**Automatic Refresh Implementation**:
```python
class TokenManager:
    def __init__(self, credentials, encryption_service, db_session):
        self.credentials = credentials
        self.encryption = encryption_service
        self.db = db_session

    async def get_valid_credentials(self):
        if self.credentials.expired and self.credentials.refresh_token:
            self.credentials.refresh(Request())
            await self.save_refreshed_credentials()
        return self.credentials

    async def save_refreshed_credentials(self):
        encrypted_creds = self.encryption.encrypt_credentials({
            'token': self.credentials.token,
            'refresh_token': self.credentials.refresh_token,
            'expiry': self.credentials.expiry.isoformat()
        })
        # Save to database...
```

**Security Best Practices**:
- Rotate encryption keys periodically
- Use secure key derivation functions (PBKDF2/scrypt) if user passwords involved
- Implement secure key storage (environment variables, key management services)
- Add credential audit logging
- Set up automatic token expiration alerts

## 3. Real-Time Email Synchronization Approaches

### Gmail Push Notifications (Recommended)

**Cloud Pub/Sub Setup**:
```python
from google.cloud import pubsub_v1
from google.oauth2.service_account import Credentials

class GmailPushNotificationHandler:
    def __init__(self, project_id, topic_name, subscription_name):
        self.project_id = project_id
        self.topic_path = f"projects/{project_id}/topics/{topic_name}"
        self.subscription_path = f"projects/{project_id}/subscriptions/{subscription_name}"

    async def setup_push_notifications(self, gmail_service, webhook_url):
        """Setup Gmail push notifications"""
        request = {
            'labelIds': ['INBOX'],
            'topicName': self.topic_path
        }
        result = gmail_service.users().watch(userId='me', body=request).execute()
        return result['historyId']

    async def handle_notification(self, message_data):
        """Process incoming push notification"""
        # Decode message and trigger sync
        email_address = message_data.get('emailAddress')
        history_id = message_data.get('historyId')
        await self.sync_changes_since(email_address, history_id)
```

**Webhook Endpoint Implementation**:
```python
from fastapi import FastAPI, Request
import base64
import json

app = FastAPI()

@app.post("/gmail/webhook/{account_id}")
async def handle_gmail_webhook(account_id: str, request: Request):
    """Handle Gmail push notification webhook"""
    body = await request.body()
    message = json.loads(body.decode())

    # Verify the message authenticity
    if not verify_webhook_signature(message):
        raise HTTPException(status_code=401, detail="Invalid signature")

    # Decode the Pub/Sub message
    data = base64.b64decode(message['message']['data'])
    notification = json.loads(data)

    # Trigger sync for this account
    await trigger_gmail_sync(account_id, notification['historyId'])

    return {"status": "processed"}
```

### Polling Fallback Strategy

**Intelligent Polling Implementation**:
```python
class GmailSyncScheduler:
    def __init__(self):
        self.account_priorities = {}
        self.last_sync_times = {}

    async def schedule_sync(self, account_id, priority='normal'):
        """Schedule sync based on account priority and activity"""
        intervals = {
            'high': 30,      # 30 seconds for active accounts
            'normal': 300,   # 5 minutes for regular accounts
            'low': 1800      # 30 minutes for inactive accounts
        }

        interval = intervals.get(priority, 300)
        await asyncio.sleep(interval)
        await self.sync_account(account_id)

    async def adaptive_scheduling(self, account_id):
        """Adjust sync frequency based on email activity"""
        recent_activity = await self.get_recent_activity(account_id)
        if recent_activity > 10:  # High activity
            priority = 'high'
        elif recent_activity > 2:
            priority = 'normal'
        else:
            priority = 'low'

        await self.schedule_sync(account_id, priority)
```

## 4. Attachment Processing Strategies

### Hybrid Smart Processing Implementation

**Format Detection and Routing**:
```python
import mimetypes
from typing import Dict, List

class AttachmentProcessor:
    IMMEDIATE_FORMATS = {
        'application/pdf': 'pdf',
        'text/plain': 'text',
        'application/vnd.openxmlformats-officedocument.wordprocessingml.document': 'docx',
        'image/jpeg': 'image',
        'image/png': 'image',
    }

    SIZE_LIMITS = {
        'immediate': 10 * 1024 * 1024,  # 10MB
        'deferred': 25 * 1024 * 1024,   # 25MB (Gmail limit)
    }

    async def process_attachment(self, attachment_data, mime_type, size):
        """Route attachment based on format and size"""
        if size > self.SIZE_LIMITS['deferred']:
            return await self.skip_attachment(attachment_data, 'too_large')

        if mime_type in self.IMMEDIATE_FORMATS and size <= self.SIZE_LIMITS['immediate']:
            return await self.process_immediately(attachment_data, mime_type)
        else:
            return await self.queue_for_background(attachment_data, mime_type)
```

**Content Extraction Pipeline**:
```python
class ContentExtractor:
    def __init__(self):
        self.extractors = {
            'pdf': self.extract_pdf,
            'docx': self.extract_docx,
            'text': self.extract_text,
            'image': self.extract_image_text,
        }

    async def extract_pdf(self, file_data):
        """Extract text from PDF using PyPDF2"""
        import PyPDF2
        import io

        pdf_reader = PyPDF2.PdfReader(io.BytesIO(file_data))
        text = ""
        for page in pdf_reader.pages:
            text += page.extract_text()
        return {'type': 'pdf', 'content': text, 'pages': len(pdf_reader.pages)}

    async def extract_docx(self, file_data):
        """Extract text from DOCX using python-docx"""
        from docx import Document
        import io

        doc = Document(io.BytesIO(file_data))
        text = "\n".join([paragraph.text for paragraph in doc.paragraphs])
        return {'type': 'docx', 'content': text}
```

**Background Processing Queue**:
```python
from celery import Celery

celery_app = Celery('penfold_attachments')

@celery_app.task
async def process_deferred_attachment(attachment_id, file_data, mime_type):
    """Background task for processing complex attachments"""
    extractor = ContentExtractor()

    try:
        content = await extractor.extract_content(file_data, mime_type)
        await save_extracted_content(attachment_id, content)
        await publish_attachment_processed_event(attachment_id, content)
    except Exception as e:
        await mark_attachment_failed(attachment_id, str(e))
```

## 5. Multi-Account Management Patterns

### Intelligent Scheduling Architecture

**Account Prioritization System**:
```python
class AccountPrioritizer:
    def __init__(self, db_session):
        self.db = db_session
        self.priority_weights = {
            'email_frequency': 0.4,
            'recent_activity': 0.3,
            'user_preference': 0.2,
            'business_hours': 0.1
        }

    async def calculate_priority(self, account_id):
        """Calculate dynamic priority for account sync"""
        metrics = await self.get_account_metrics(account_id)

        frequency_score = min(metrics['daily_emails'] / 100, 1.0)
        activity_score = min(metrics['recent_emails'] / 10, 1.0)
        preference_score = metrics['user_priority'] / 5.0
        business_score = 1.0 if self.is_business_hours() else 0.5

        priority = (
            frequency_score * self.priority_weights['email_frequency'] +
            activity_score * self.priority_weights['recent_activity'] +
            preference_score * self.priority_weights['user_preference'] +
            business_score * self.priority_weights['business_hours']
        )

        return min(max(priority, 0.1), 1.0)  # Clamp between 0.1 and 1.0
```

**Parallel Processing with Resource Management**:
```python
class MultiAccountSyncManager:
    def __init__(self, max_concurrent_accounts=3):
        self.account_semaphore = asyncio.Semaphore(max_concurrent_accounts)
        self.rate_limiters = {}

    async def sync_account(self, account_id):
        """Sync single account with resource management"""
        async with self.account_semaphore:
            rate_limiter = self.get_rate_limiter(account_id)
            async with rate_limiter:
                await self.perform_sync(account_id)

    async def sync_all_accounts(self, account_ids):
        """Sync multiple accounts with priority ordering"""
        priorities = await self.get_account_priorities(account_ids)
        sorted_accounts = sorted(account_ids, key=lambda x: priorities[x], reverse=True)

        # Process high priority accounts in parallel, others sequentially
        high_priority = sorted_accounts[:3]
        normal_priority = sorted_accounts[3:]

        # Parallel processing for high priority
        await asyncio.gather(*[self.sync_account(aid) for aid in high_priority])

        # Sequential processing for normal priority
        for account_id in normal_priority:
            await self.sync_account(account_id)
```

## 6. Event Publishing Integration

### Event Schema Design

**Email Content Events**:
```python
from pydantic import BaseModel
from datetime import datetime
from typing import List, Optional, Dict, Any

class EmailIngestedEvent(BaseModel):
    event_type: str = "content.ingested"
    source_type: str = "gmail"
    timestamp: datetime
    account_id: str
    email_id: str
    thread_id: str

    # Email metadata
    subject: str
    sender: str
    recipients: List[str]
    sent_at: datetime

    # Content
    body_text: str
    body_html: Optional[str]
    attachments: List[Dict[str, Any]]

    # Threading
    in_reply_to: Optional[str]
    references: List[str]

    # Labels and categorization
    labels: List[str]
    importance: str

    class Config:
        json_encoders = {datetime: lambda v: v.isoformat()}
```

### Publisher Implementation

**Redis Pub/Sub Event Publishing**:
```python
import redis.asyncio as redis
import json

class GmailEventPublisher:
    def __init__(self, redis_url):
        self.redis = redis.from_url(redis_url)

    async def publish_email_ingested(self, email_event: EmailIngestedEvent):
        """Publish email ingested event"""
        channel = f"gmail.{email_event.account_id}.ingested"
        message = email_event.json()

        await self.redis.publish(channel, message)

        # Also publish to general ingestion channel
        await self.redis.publish("content.ingested", message)

    async def publish_sync_completed(self, account_id: str, stats: Dict):
        """Publish sync completion event"""
        event = {
            'event_type': 'gmail.sync.completed',
            'account_id': account_id,
            'timestamp': datetime.utcnow().isoformat(),
            'stats': stats
        }

        await self.redis.publish(f"gmail.{account_id}.sync", json.dumps(event))
```

## 7. Database Schema Patterns

### Core Models

**Gmail Connection Model**:
```python
from sqlalchemy import Column, String, DateTime, Text, Boolean, JSON
from sqlalchemy.ext.declarative import declarative_base
from sqlalchemy.dialects.postgresql import UUID
import uuid

Base = declarative_base()

class GmailConnection(Base):
    __tablename__ = 'gmail_connections'

    id = Column(UUID(as_uuid=True), primary_key=True, default=uuid.uuid4)
    user_id = Column(String, nullable=False, index=True)
    account_email = Column(String, nullable=False)

    # Encrypted credentials
    encrypted_credentials = Column(Text, nullable=False)
    credentials_updated_at = Column(DateTime, nullable=False)

    # Sync state
    last_history_id = Column(String)
    last_sync_at = Column(DateTime)
    sync_enabled = Column(Boolean, default=True)

    # Configuration
    privacy_config = Column(JSON)
    sync_config = Column(JSON)

    created_at = Column(DateTime, nullable=False)
    updated_at = Column(DateTime, nullable=False)
```

**Email Message Model**:
```python
class EmailMessage(Base):
    __tablename__ = 'email_messages'

    id = Column(UUID(as_uuid=True), primary_key=True, default=uuid.uuid4)
    gmail_connection_id = Column(UUID(as_uuid=True), ForeignKey('gmail_connections.id'))

    # Gmail identifiers
    gmail_message_id = Column(String, nullable=False, unique=True)
    gmail_thread_id = Column(String, nullable=False, index=True)

    # Message content
    subject = Column(String)
    sender = Column(String, nullable=False)
    recipients = Column(JSON)  # List of recipients
    sent_at = Column(DateTime, nullable=False, index=True)

    body_text = Column(Text)
    body_html = Column(Text)

    # Metadata
    labels = Column(JSON)  # Gmail labels
    importance = Column(String)

    # Processing state
    processed_at = Column(DateTime)
    processing_status = Column(String, default='pending')

    created_at = Column(DateTime, nullable=False)
```

## 8. Implementation Recommendations

### Development Phases

**Phase 1**: Basic OAuth2 and API Connection
- Implement OAuth2 flow with credential encryption
- Basic Gmail API client with rate limiting
- Simple message fetching and storage

**Phase 2**: Real-time Synchronization
- Push notification setup and webhook handling
- Incremental sync with history API
- Event publishing integration

**Phase 3**: Attachment Processing
- Immediate processing for common formats
- Background queue for complex formats
- Content extraction and indexing

**Phase 4**: Multi-Account and Privacy
- Multiple account management
- Privacy filtering implementation
- Account prioritization and scheduling

### Testing Strategy

**Unit Tests**:
- OAuth2 flow mocking
- Rate limiting behavior
- Encryption/decryption
- Event publishing

**Integration Tests**:
- Gmail API interaction with test account
- Database operations with async fixtures
- End-to-end sync workflows

**Performance Tests**:
- Rate limit compliance under load
- Large email volume handling
- Concurrent account processing

### Security Considerations

**Data Protection**:
- Encrypt all OAuth2 credentials at rest
- Implement secure key management
- Add audit logging for credential access
- Regular credential rotation

**Privacy Controls**:
- Configurable email exclusion filters
- Content scanning controls
- Data retention policies
- User consent management

**API Security**:
- Request signing for webhooks
- Rate limiting enforcement
- Error handling without data leakage
- Secure token refresh flows

This research provides the foundation for implementing a secure, performant Gmail integration that follows industry best practices while meeting the specific requirements of the Penfold system.