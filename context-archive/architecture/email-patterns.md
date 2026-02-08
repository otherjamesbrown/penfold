# Email Integration Patterns

> **Note**: Code examples are from the original Python implementation for reference. Go implementations are in `services/gmail/`.

## 18. OAuth2 Token Management with Encryption

**Pattern**: Secure storage and lifecycle management for OAuth2 credentials

**Implementation Details**:
- AES-256-GCM encryption for token storage at rest
- Automatic token refresh before expiration
- Revocation detection and re-authorization flow
- Multi-account credential isolation

**Go Implementation**: `services/gmail/oauth/`

## 19. Real-Time Sync with Push/Poll Fallback

**Pattern**: Hybrid synchronization using push notifications with polling fallback

**Implementation Details**:
- Gmail Push notifications via Cloud Pub/Sub webhooks
- Automatic fallback to polling if push fails
- Configurable sync intervals for fallback mode
- Catch-up sync after connectivity restoration

**Go Implementation**: `services/gmail/push/`, `services/gmail/sync/`

## 20. Multi-Account Priority Scheduling

**Pattern**: Intelligent scheduling across multiple accounts with priority management

**Implementation Details**:
- Priority-based account ordering (work > personal)
- Resource allocation based on account priority
- Concurrent sync limits to prevent overload
- Cross-account deduplication

**Go Implementation**: `services/gmail/scheduler/`

## 21. Attachment Processing Pipeline

**Pattern**: Background queue processing for email attachments with format handling

**Implementation Details**:
- Task queue for background processing (Temporal activities)
- Format-specific extractors (PDF, DOCX, images)
- Size limits and validation
- Retry logic for transient failures

**Go Implementation**: `services/gmail/attachment/`

## 22. Privacy Filter Chain

**Pattern**: Configurable filtering pipeline for sensitive content

**Implementation Details**:
- Chain of responsibility for filter evaluation
- Label-based sensitivity detection
- Configurable exclusion rules
- Audit logging for filtered content

---

## Email Integration Performance

### Sync Performance
- **Real-time Detection**: <60 seconds for new email notification
- **Historical Import**: 100-150 emails/minute with rate limit compliance
- **Attachment Processing**: Background queue, <5 minutes for common formats

### Rate Limit Management
- **Gmail API Quota**: 250 units/user/second, 1B units/day
- **Token Bucket**: Smooth request distribution across quota window
- **Backoff Strategy**: Exponential backoff on 429 responses

### Multi-Account Efficiency
- **Concurrent Syncs**: Up to 3 accounts simultaneously
- **Priority Scheduling**: High-priority accounts sync more frequently
- **Resource Isolation**: Failures in one account don't affect others
