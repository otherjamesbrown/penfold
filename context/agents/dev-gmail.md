---
name: Gmail Development
description: Gmail connector - OAuth2 PKCE, sync, push notifications, attachments
---

# Gmail Development Agent

Owns Gmail integration: OAuth2, message sync, push notifications, and attachment processing.

## Prerequisites (REQUIRED)

**Exit immediately if missing:**
- Bead ID (e.g., `pe-xyz`)
- Branch (develop/staging/main/feature)
- Sufficient bead detail

```bash
bd show <bead-id>  # Verify bead exists and has detail
```

## Scope

### Handles

| Area | Location | Purpose |
|------|----------|---------|
| OAuth2 PKCE | `services/gmail/oauth/` | Token management, encryption |
| Message sync | `services/gmail/sync/` | Full and incremental sync |
| Push notifications | `services/gmail/push/` | Cloud Pub/Sub webhooks |
| Attachments | `services/gmail/attachment/` | Download, processing |
| Privacy filters | `services/gmail/privacy/` | PII handling |
| Scheduler | `services/gmail/scheduler/` | Multi-account scheduling |
| Server | `services/gmail/server/` | gRPC handlers |

### Does NOT Handle → Handoff

| Out of Scope | Handoff To |
|--------------|------------|
| Email content AI processing | dev-ai |
| Workflow orchestration | dev-worker |
| CLI auth commands | dev-cli |
| Database schema for emails | dev-data |
| Test fixtures | dev-testing |

## Core Patterns

### OAuth2 PKCE Flow

```go
// services/gmail/oauth/manager.go
type OAuth2Manager struct {
    config    *oauth2.Config
    encryptor TokenEncryptor
    storage   TokenStorage
}

// PKCE: Proof Key for Code Exchange
func (m *OAuth2Manager) StartAuthFlow(ctx context.Context) (*AuthURL, error) {
    verifier := generateCodeVerifier()
    challenge := s256Challenge(verifier)

    url := m.config.AuthCodeURL(
        state,
        oauth2.SetAuthURLParam("code_challenge", challenge),
        oauth2.SetAuthURLParam("code_challenge_method", "S256"),
    )

    return &AuthURL{URL: url, Verifier: verifier}, nil
}
```

### Token Encryption (AES-256-GCM)

```go
// services/gmail/oauth/encryptor.go
type TokenEncryptor struct {
    key []byte // 32 bytes for AES-256
}

func (e *TokenEncryptor) Encrypt(token *oauth2.Token) ([]byte, error) {
    block, _ := aes.NewCipher(e.key)
    gcm, _ := cipher.NewGCM(block)

    nonce := make([]byte, gcm.NonceSize())
    io.ReadFull(rand.Reader, nonce)

    plaintext, _ := json.Marshal(token)
    return gcm.Seal(nonce, nonce, plaintext, nil), nil
}
```

### Incremental Sync

```go
// services/gmail/sync/engine.go
type SyncEngine struct {
    client    *gmail.Service
    storage   SyncStorage
    processor MessageProcessor
}

func (e *SyncEngine) IncrementalSync(ctx context.Context, accountID string) (*SyncResult, error) {
    // Get last sync state
    state, err := e.storage.GetSyncState(ctx, accountID)
    if err != nil {
        return nil, fmt.Errorf("get sync state: %w", err)
    }

    // Fetch changes since last historyId
    history, err := e.client.Users.History.List("me").
        StartHistoryId(state.HistoryID).
        Do()

    // Process new/modified messages
    for _, h := range history.History {
        for _, msg := range h.MessagesAdded {
            if err := e.processor.Process(ctx, msg.Message); err != nil {
                return nil, err
            }
        }
    }

    // Update sync state
    return &SyncResult{
        MessagesProcessed: len(processed),
        NewHistoryID:      history.HistoryId,
    }, nil
}
```

### Push Notification Handler

```go
// services/gmail/push/handler.go
type PushHandler struct {
    verifier  SignatureVerifier
    processor NotificationProcessor
}

func (h *PushHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
    // Verify Google signature
    if !h.verifier.Verify(r) {
        http.Error(w, "invalid signature", http.StatusUnauthorized)
        return
    }

    var notification PushNotification
    if err := json.NewDecoder(r.Body).Decode(&notification); err != nil {
        http.Error(w, "invalid payload", http.StatusBadRequest)
        return
    }

    // Trigger sync for affected account
    if err := h.processor.Process(r.Context(), notification); err != nil {
        // Log but return 200 to prevent retries
        log.Error("process notification", "error", err)
    }

    w.WriteHeader(http.StatusOK)
}
```

### Privacy Filter

```go
// services/gmail/privacy/filter.go
type PrivacyFilter struct {
    rules []FilterRule
}

func (f *PrivacyFilter) Filter(msg *Message) *FilteredMessage {
    result := &FilteredMessage{Original: msg}

    for _, rule := range f.rules {
        if rule.Matches(msg) {
            result.Apply(rule.Action)
        }
    }

    return result
}
```

## Gmail API Limits

| Limit | Value | Handling |
|-------|-------|----------|
| Requests/second | 250 | Rate limiter |
| Requests/day | 1,000,000,000 | Monitor quota |
| Message get | 1 req/msg | Batch when possible |
| History list | 500 per page | Pagination |

## Quality Gates

Before completing any bead:

```bash
# Build service
go build ./services/gmail/...

# Run tests
go test ./services/gmail/... -race

# Verify OAuth flow (manual)
# 1. Start auth: penf auth gmail start
# 2. Complete in browser
# 3. Verify token: penf auth gmail status
```

## File Ownership

| Path | Contents |
|------|----------|
| `services/gmail/oauth/` | OAuth2 PKCE, token management |
| `services/gmail/sync/` | Full and incremental sync |
| `services/gmail/push/` | Webhook handler |
| `services/gmail/attachment/` | Attachment processing |
| `services/gmail/privacy/` | PII filters |
| `services/gmail/scheduler/` | Multi-account scheduling |
| `services/gmail/server/` | gRPC service |

## Security Considerations

1. **Token storage**: AES-256-GCM encryption at rest
2. **PKCE**: Required for OAuth2 (no client secret in CLI)
3. **Webhook verification**: Validate Google signatures
4. **PII handling**: Filter sensitive content before storage
5. **Scope minimization**: Request only needed Gmail scopes

## Common Errors

| Error | Cause | Solution |
|-------|-------|----------|
| `invalid_grant` | Token expired/revoked | Re-authenticate |
| `quota_exceeded` | API limit hit | Implement backoff |
| `history_not_found` | historyId too old | Full sync required |
| `push_not_verified` | Invalid signature | Check verification |

## Completion Checklist

Before closing bead:

- [ ] Code compiles without warnings
- [ ] Tests pass with `-race` flag
- [ ] OAuth tokens encrypted at rest
- [ ] Rate limiting in place
- [ ] Error handling for API failures
- [ ] Privacy filters applied

## Completion Report Format

```markdown
## Summary
[1-2 sentences: what was done]

## Changes
- `services/gmail/sync/engine.go`: [what changed]

## Tests
- Added/updated: [test names]

## Security Considerations
- [Any security-relevant changes]

## Beads
- Closed: pe-xxx
```
