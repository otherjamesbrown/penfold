# Gmail Integration Setup Guide

This guide walks you through setting up Gmail integration with Penfold for secure email processing and analysis.

## Prerequisites

- Go 1.22 or later
- PostgreSQL 16+ with pgvector extension
- Temporal server (for workflow orchestration)
- Google Cloud Console project with Gmail API access
- Gmail account with appropriate permissions

## Overview

Gmail integration enables Penfold to:
- Securely connect to your Gmail account using OAuth2 with PKCE
- Import historical emails for analysis
- Monitor new emails in real-time via Cloud Pub/Sub
- Extract and process email attachments
- Apply privacy filters to protect sensitive content
- Support multiple Gmail accounts

## Step 1: Google Cloud Console Setup

### 1.1 Create Google Cloud Project

1. Go to [Google Cloud Console](https://console.cloud.google.com/)
2. Create a new project or select an existing one
3. Note your Project ID for later configuration

### 1.2 Enable Gmail API

1. Navigate to **APIs & Services > Library**
2. Search for "Gmail API"
3. Click on Gmail API and press **Enable**

### 1.3 Configure OAuth2 Consent Screen

1. Go to **APIs & Services > OAuth consent screen**
2. Choose **External** user type (unless using Google Workspace)
3. Fill in required fields:
   - **App name**: "Penfold Gmail Integration"
   - **User support email**: Your email address
   - **Developer contact email**: Your email address
4. Add scopes:
   - `https://www.googleapis.com/auth/gmail.readonly`
   - `https://www.googleapis.com/auth/gmail.modify`
   - `https://www.googleapis.com/auth/gmail.labels`
5. Add test users (your Gmail address) if in testing mode

### 1.4 Create OAuth2 Credentials

1. Go to **APIs & Services > Credentials**
2. Click **+ CREATE CREDENTIALS > OAuth 2.0 Client IDs**
3. Choose **Desktop application**
4. Name it "Penfold Gmail Client"
5. Download the credentials JSON file
6. Save it to a secure location (e.g., `~/.penfold/gmail_credentials.json`)

## Step 2: Build and Configure Penfold

### 2.1 Build the Gmail Service

```bash
# Clone the repository (if not already done)
git clone https://github.com/otherjamesbrown/penfold.git
cd penfold

# Build the Gmail connector service
go build -o bin/gmail-connector ./services/gmail

# Build the CLI tool
go build -o bin/penf ./cmd/penf
```

### 2.2 Configure Environment Variables

Set up the required environment variables:

```bash
# Gmail service configuration
export GMAIL_GRPC_PORT=50051
export GMAIL_HTTP_PORT=8081
export GMAIL_OAUTH_CREDENTIALS_PATH=/path/to/gmail_credentials.json
export GMAIL_TOKEN_STORE_PATH=/path/to/tokens
export GMAIL_MAX_SYNC_BATCH_SIZE=500
export GMAIL_SYNC_TIMEOUT_SECONDS=300

# Encryption key for token storage (32 bytes, base64 encoded)
# Generate with: openssl rand -base64 32
export GMAIL_ENCRYPTION_KEY="your-base64-encoded-32-byte-key"

# Database configuration
export DATABASE_URL="postgres://user:pass@localhost:5432/penfold"

# Temporal configuration
export TEMPORAL_HOST_PORT="localhost:7233"
export TEMPORAL_NAMESPACE="penfold"
```

### 2.3 Configure YAML Settings (Optional)

Create or update your Penfold configuration file:

```yaml
# ~/.penfold/config.yaml

gmail:
  enabled: true
  credentials_file: "~/.penfold/gmail_credentials.json"

  # Service ports
  grpc_port: 50051
  http_port: 8081

  # Sync configuration
  batch_size: 100
  sync_timeout_seconds: 300

  # Real-time monitoring (requires Cloud Pub/Sub setup)
  realtime_sync: true
  pubsub_topic: "projects/your-project/topics/gmail-notifications"
  pubsub_subscription: "projects/your-project/subscriptions/gmail-penfold"
  polling_fallback: true
  polling_interval: 300  # seconds

  # Privacy and filtering
  privacy:
    sensitivity_level: "medium"  # low, medium, high
    redaction_placeholder: "[REDACTED]"
    blocked_senders: []
    blocked_domains: []
    allowed_domains: []

  # Attachment processing
  attachments:
    enabled: true
    max_size_mb: 25
    concurrent_limit: 10
    supported_formats:
      - "application/pdf"
      - "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
      - "text/plain"
      - "image/jpeg"
      - "image/png"
```

## Step 3: Initial Gmail Connection

### 3.1 Start the Gmail Service

```bash
# Start the Gmail connector service
./bin/gmail-connector

# Expected output:
# INFO starting Gmail Connector service grpc_port=50051 http_port=8081
# INFO starting HTTP server for health and metrics address=:8081
# INFO starting gRPC server address=:50051
```

### 3.2 Authenticate with Gmail

Run the Gmail connection command:

```bash
penf gmail connect
```

This will:
1. Generate a PKCE code verifier and challenge
2. Open your default web browser
3. Redirect you to Google's OAuth2 authorization page
4. Ask you to sign in to your Gmail account
5. Request permission to access your emails
6. Return an authorization code
7. Exchange the code for tokens (with PKCE verification)
8. Store encrypted credentials securely

### 3.3 Verify Connection

Test the connection:

```bash
penf gmail status
```

Expected output:
```
Gmail Connection Status
=======================
Account: your.email@gmail.com
Status: Active
Last Sync: Never (new connection)
Token Expires: 2026-02-13 14:30:00
API Quota: 1,000,000,000 units remaining
```

## Step 4: Historical Email Import

### 4.1 Configure Import Scope

Review and adjust import settings:

```bash
penf gmail config --list
```

To modify import range:

```bash
# Import last 30 days only
penf gmail config --import-days-back 30

# Import from specific date
penf gmail config --import-from-date "2025-01-01"
```

### 4.2 Start Historical Import

Begin importing historical emails:

```bash
# Preview what will be imported
penf gmail import --dry-run

# Start actual import
penf gmail import
```

Monitor import progress:

```bash
penf gmail import --status
```

Expected output:
```
Historical Import Status
========================
Total Emails:    2,347
Processed:       1,892
Remaining:       455
Rate:            89/min
ETA:             5 minutes
Errors:          3

Recent Activity:
  Thread: "Project Alpha Updates" (12 messages)
  Email: "Weekly Team Standup Notes"
  Attachment deferred: large_presentation.pptx (25MB)
```

## Step 5: Real-Time Monitoring

### 5.1 Configure Cloud Pub/Sub (Required for Push Notifications)

1. In Google Cloud Console, go to **Pub/Sub > Topics**
2. Create a topic named `gmail-notifications`
3. Create a subscription named `gmail-penfold`
4. Configure push delivery to your webhook endpoint

```bash
# Create topic
gcloud pubsub topics create gmail-notifications

# Create subscription with push endpoint
gcloud pubsub subscriptions create gmail-penfold \
  --topic=gmail-notifications \
  --push-endpoint=https://your-domain.com/webhooks/gmail \
  --ack-deadline=60
```

### 5.2 Set Up Gmail Watch

Enable Gmail push notifications for your account:

```bash
penf gmail watch --setup
```

This configures Gmail to send notifications to your Pub/Sub topic when new emails arrive.

### 5.3 Verify Real-Time Processing

Send yourself a test email and verify processing:

```bash
penf gmail monitor --test-email
```

Expected output:
```
Test Email Sent: test-20260123-143052@gmail.com
Detection Time: 23 seconds
Processing Time: 4 seconds
Total Latency: 27 seconds (target: <60s)
```

### 5.4 Polling Fallback

If push notifications are not available, enable polling fallback:

```bash
penf gmail config --polling-fallback true --polling-interval 300
```

## Step 6: Multiple Account Setup

### 6.1 Add Additional Accounts

To connect multiple Gmail accounts:

```bash
penf gmail connect --account work.email@company.com
penf gmail connect --account personal@gmail.com
```

### 6.2 Configure Account Priorities

Set sync priorities for multiple accounts:

```bash
# High priority - sync every 60 seconds
penf gmail config --account work.email@company.com --priority high

# Medium priority - sync every 300 seconds
penf gmail config --account personal@gmail.com --priority medium

# Low priority - sync every 1800 seconds
penf gmail config --account archive@gmail.com --priority low
```

### 6.3 View All Accounts

List all connected accounts:

```bash
penf gmail accounts
```

Expected output:
```
Connected Gmail Accounts
========================
Account                      Priority   Last Sync    Status
---------------------------  ---------  -----------  --------
work.email@company.com       High       2min ago     Active
personal@gmail.com           Medium     8min ago     Active
archive@gmail.com            Low        45min ago    Active
```

## Step 7: Privacy Configuration

### 7.1 Configure Privacy Filters

Gmail integration supports three sensitivity levels:

```bash
# Low - only blocklist filtering
penf gmail config --privacy-level low

# Medium - PII detection and redaction (default)
penf gmail config --privacy-level medium

# High - full content filtering including body analysis
penf gmail config --privacy-level high
```

### 7.2 Configure Blocklists

```bash
# Block specific senders
penf gmail config --block-sender spam@example.com

# Block entire domains
penf gmail config --block-domain spam.example.com

# Allowlist trusted domains (bypass PII filtering)
penf gmail config --allow-domain company.com
```

### 7.3 Credential Security

Your Gmail credentials are protected through:
- **AES-256-GCM encryption** for stored OAuth2 tokens
- **PKCE** for authorization code flow security
- **Automatic token refresh** without exposing secrets
- **No password storage** - only OAuth2 tokens

To rotate credentials:

```bash
# Revoke current access
penf gmail disconnect --account your.email@gmail.com

# Re-authenticate with fresh tokens
penf gmail connect --account your.email@gmail.com
```

## Step 8: Health Monitoring

### 8.1 Service Health Endpoints

The Gmail service exposes health endpoints:

```bash
# Overall health
curl http://localhost:8081/health

# Readiness check
curl http://localhost:8081/ready

# Liveness check
curl http://localhost:8081/live

# Prometheus metrics
curl http://localhost:8081/metrics
```

### 8.2 Check Service Status

```bash
penf gmail status --verbose
```

## Troubleshooting

### Common Issues

**Authentication Errors**
```
Error: OAuth2 token expired
Solution: penf gmail refresh-auth
```

**Rate Limiting**
```
Error: Gmail API quota exceeded
Solution: Wait 24 hours or reduce batch size
Command: penf gmail config --batch-size 50
```

**Missing Emails**
```
Issue: New emails not detected
Solution: Check real-time monitoring status
Command: penf gmail monitor --status
```

**Attachment Processing Failures**
```
Issue: Large attachments failing to process
Solution: Increase timeout or reduce max size
Command: penf gmail config --attachment-timeout 600
```

### Support and Logs

View detailed logs:
```bash
# Service logs (from service startup)
./bin/gmail-connector 2>&1 | tee gmail.log

# Check specific operations
penf gmail logs --tail
penf gmail logs --level debug
```

Generate diagnostic report:
```bash
penf gmail diagnostic --export /tmp/gmail-diagnostic.zip
```

For additional support, see [troubleshooting guide](./troubleshooting.md).

## Next Steps

Once Gmail integration is set up:

1. **Review Processing Results**: `penf review --daily`
2. **Query Your Emails**: `penf ask "What emails mention the Atlas project?"`
3. **Explore Timelines**: `penf timeline --source gmail --last 30d`
4. **Configure AI Processing**: See [AI Architecture Guide](../ai-architecture.md)

## Security Best Practices

1. **Regular Token Rotation**: Refresh OAuth2 tokens monthly
2. **Privacy Audits**: Review excluded content quarterly
3. **Access Monitoring**: Check `penf gmail audit` for unusual activity
4. **Minimal Permissions**: Only grant necessary Gmail API scopes
5. **Secure Storage**: Ensure config files have restricted permissions

```bash
# Secure your configuration
chmod 600 ~/.penfold/config.yaml
chmod 600 ~/.penfold/gmail_credentials.json
chmod 700 ~/.penfold/tokens/
```

## Reference: Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `GMAIL_GRPC_PORT` | gRPC service port | 50051 |
| `GMAIL_HTTP_PORT` | Health/metrics port | 8081 |
| `GMAIL_OAUTH_CREDENTIALS_PATH` | Path to OAuth credentials JSON | Required |
| `GMAIL_TOKEN_STORE_PATH` | Path to token storage directory | Required |
| `GMAIL_ENCRYPTION_KEY` | Base64-encoded 32-byte encryption key | Required |
| `GMAIL_MAX_SYNC_BATCH_SIZE` | Max emails per sync batch | 500 |
| `GMAIL_SYNC_TIMEOUT_SECONDS` | Sync operation timeout | 300 |
