# Gmail Connector Specification

## Overview

The Gmail Connector handles OAuth2 authentication, email synchronization, and attachment processing for Gmail accounts.

## Status: Planned (Phase 3)

## Responsibilities

1. **OAuth2 Management**: Token storage, refresh, revocation
2. **Email Sync**: Historical and incremental via Gmail History API
3. **Real-time**: Push notifications via Google Pub/Sub
4. **Attachments**: Download, extract content, deduplicate
5. **Multi-account**: Support multiple Gmail accounts per tenant
6. **Rate Limiting**: Respect Gmail API quotas (250 req/sec)

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Gmail Connector                          │
│                                                              │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐  │
│  │ OAuth Manager│    │ Sync Engine  │    │ Attachment   │  │
│  │              │    │              │    │ Processor    │  │
│  └──────────────┘    └──────────────┘    └──────────────┘  │
│         │                   │                   │           │
│         ▼                   ▼                   ▼           │
│  ┌─────────────────────────────────────────────────────┐   │
│  │                  Gmail API Client                    │   │
│  │     (google.golang.org/api/gmail/v1)                │   │
│  └─────────────────────────────────────────────────────┘   │
│                            │                                │
└────────────────────────────┼────────────────────────────────┘
                             ▼
                      ┌──────────────┐
                      │  Gmail API   │
                      └──────────────┘
```

## gRPC Service

```protobuf
service GmailService {
  // Account management
  rpc ConnectAccount(ConnectAccountRequest) returns (ConnectAccountResponse);
  rpc DisconnectAccount(DisconnectAccountRequest) returns (DisconnectAccountResponse);
  rpc ListAccounts(ListAccountsRequest) returns (ListAccountsResponse);

  // Sync operations
  rpc TriggerSync(TriggerSyncRequest) returns (TriggerSyncResponse);
  rpc GetSyncStatus(GetSyncStatusRequest) returns (GetSyncStatusResponse);

  // Health
  rpc Health(HealthRequest) returns (HealthResponse);
}
```

## Key Features

### OAuth2 PKCE Flow
- Secure public client authentication
- AES-256 encrypted token storage
- Automatic token refresh

### Sync Strategies
- **Full Sync**: Initial historical import
- **Incremental Sync**: Gmail History API for changes
- **Real-time**: Push notifications + polling fallback

### Privacy Filters
- Label-based exclusion
- Sender/domain filtering
- Content pattern matching

## Configuration

```yaml
oauth:
  client_id: "${GMAIL_CLIENT_ID}"
  client_secret: "${GMAIL_CLIENT_SECRET}"
  redirect_uri: "http://localhost:8088/oauth/callback"

sync:
  batch_size: 100
  max_concurrent: 5
  history_days: 365  # Initial sync depth

rate_limit:
  requests_per_second: 200
  burst_size: 50

attachments:
  max_size_mb: 25
  storage_path: "/data/attachments"
```

## Events Published

- `email.ingested` - New email processed
- `email.thread_ingested` - Thread updated
- `email.attachment_ingested` - Attachment processed
- `sync.progress` - Sync progress update
- `sync.completed` - Sync operation finished
