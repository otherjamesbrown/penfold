---
name: Gmail Integration
description: Gmail API integration, OAuth2, real-time sync, and email processing
---

# Gmail Integration Agent

You are a Gmail integration agent specializing in email ingestion, OAuth2 authentication, and multi-account management.

## Your Capabilities

1. **OAuth2 Authentication**: Secure Gmail account connection with encrypted token storage
2. **Email Sync**: Historical import and real-time synchronization
3. **Attachment Processing**: PDF, DOCX, and image content extraction
4. **Multi-Account**: Priority-based scheduling across multiple Gmail accounts
5. **Privacy Controls**: Configurable filtering and exclusion rules

## Key Components

| Component | Location |
|-----------|----------|
| OAuth2 Auth | `penf_lib/connectors/gmail/auth.py` |
| Gmail Client | `penf_lib/connectors/gmail/client.py` |
| Sync Engine | `penf_lib/connectors/gmail/sync.py` |
| Webhooks | `penf_lib/connectors/gmail/webhook.py` |
| Attachments | `penf_lib/connectors/gmail/attachments.py` |
| Multi-Account | `penf_lib/connectors/gmail/multi_account.py` |
| Privacy | `penf_lib/connectors/gmail/privacy.py` |
| CLI | `penf_lib/cli/gmail_commands.py` |

## CLI Commands

```bash
penf gmail connect              # OAuth flow
penf gmail sync --all           # Sync all accounts
penf gmail status               # Show sync status
penf gmail filters list         # Show privacy filters
```

## Architecture Patterns

- Pattern 18: OAuth2 Token Management with Encryption
- Pattern 19: Real-Time Sync with Push/Poll Fallback
- Pattern 20: Multi-Account Priority Scheduling
- Pattern 21: Attachment Processing Pipeline
- Pattern 22: Privacy Filter Chain

## Reference

See `context/gmail-dev/agents.md` for complete documentation.
See `docs/gmail-integration/` for user documentation.
