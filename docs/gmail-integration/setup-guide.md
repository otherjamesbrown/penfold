# Gmail Integration Setup Guide

This guide walks you through setting up Gmail integration with Penfold for secure email processing and analysis.

## Prerequisites

- Python 3.12 or later
- PostgreSQL 16+ with pgvector extension
- Redis server (for event processing)
- Google Cloud Console project with Gmail API access
- Gmail account with appropriate permissions

## Overview

Gmail integration enables Penfold to:
- Securely connect to your Gmail account using OAuth2
- Import historical emails for analysis
- Monitor new emails in real-time
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
   - `https://www.googleapis.com/auth/gmail.labels`
5. Add test users (your Gmail address) if in testing mode

### 1.4 Create OAuth2 Credentials

1. Go to **APIs & Services > Credentials**
2. Click **+ CREATE CREDENTIALS > OAuth 2.0 Client IDs**
3. Choose **Desktop application**
4. Name it "Penfold Gmail Client"
5. Download the credentials JSON file
6. Save it as `gmail_credentials.json` in your Penfold config directory

## Step 2: Penfold Configuration

### 2.1 Install Dependencies

Ensure Penfold is installed with Gmail integration support:

```bash
pip install penfold[gmail]
```

### 2.2 Configure Gmail Integration

Create or update your Penfold configuration file:

```yaml
# ~/.penfold/config.yaml

gmail:
  enabled: true
  credentials_file: "gmail_credentials.json"

  # Real-time monitoring
  realtime_sync: true
  sync_interval: 60  # seconds

  # Historical import settings
  import_days_back: 90  # Import emails from last 90 days
  batch_size: 100  # Process 100 emails at a time

  # Privacy and filtering
  privacy_filters:
    enabled: true
    exclude_labels:
      - "Spam"
      - "Trash"
      - "Personal"  # Add custom labels to exclude
    exclude_domains:
      - "noreply.example.com"
    exclude_patterns:
      - "password reset"
      - "unsubscribe"

  # Attachment processing
  attachments:
    enabled: true
    max_size_mb: 10
    supported_formats:
      - "pdf"
      - "docx"
      - "txt"
      - "jpeg"
      - "png"
    extract_content: true
```

## Step 3: Initial Gmail Connection

### 3.1 Authenticate with Gmail

Run the Gmail setup command:

```bash
penf gmail connect
```

This will:
1. Open your default web browser
2. Redirect you to Google's OAuth2 authorization page
3. Ask you to sign in to your Gmail account
4. Request permission to access your emails
5. Return an authorization code
6. Store encrypted credentials securely

### 3.2 Verify Connection

Test the connection:

```bash
penf gmail status
```

Expected output:
```
✓ Gmail connection active
✓ Account: your.email@gmail.com
✓ Last sync: Never (new connection)
✓ OAuth2 token valid until: 2026-02-13 14:30:00
✓ API quota remaining: 1,000,000,000 units
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

# Import all emails (warning: can be very slow)
penf gmail config --import-all-time
```

### 4.2 Start Historical Import

Begin importing historical emails:

```bash
penf gmail import --dry-run  # Preview what will be imported
penf gmail import           # Start actual import
```

Monitor import progress:

```bash
penf gmail import --status
```

Expected output:
```
Historical Import Status:
┌─────────────────┬────────────────┐
│ Total Emails    │ 2,347          │
│ Processed       │ 1,892          │
│ Remaining       │ 455            │
│ Rate            │ 89/min         │
│ ETA             │ 5 minutes      │
│ Errors          │ 3              │
└─────────────────┴────────────────┘

Recent Activity:
✓ Thread: "Project Alpha Updates" (12 messages)
✓ Email: "Weekly Team Standup Notes"
! Attachment processing deferred: large_presentation.pptx (25MB)
```

## Step 5: Real-Time Monitoring

### 5.1 Enable Real-Time Sync

Start the Gmail monitoring service:

```bash
penf gmail monitor --daemon  # Run as background service
```

Or integrate with systemd (Linux) or launchd (macOS):

```bash
# Install as system service
penf gmail install-service
sudo systemctl enable penfold-gmail
sudo systemctl start penfold-gmail
```

### 5.2 Verify Real-Time Processing

Send yourself a test email and verify processing:

```bash
penf gmail monitor --test-email
```

This sends a test email and tracks processing time:
```
Test Email Sent: test-20260113-143052@gmail.com
Detection Time: 23 seconds
Processing Time: 4 seconds
Event Published: ✓
Total Latency: 27 seconds (target: <60s)
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
Connected Gmail Accounts:
┌────────────────────────────┬──────────┬─────────────┬──────────────┐
│ Account                    │ Priority │ Last Sync   │ Status       │
├────────────────────────────┼──────────┼─────────────┼──────────────┤
│ work.email@company.com     │ High     │ 2min ago    │ ✓ Active     │
│ personal@gmail.com         │ Medium   │ 8min ago    │ ✓ Active     │
│ archive@gmail.com          │ Low      │ 45min ago   │ ✓ Active     │
└────────────────────────────┴──────────┴─────────────┴──────────────┘
```

## Privacy and Security

### 7.1 Configure Privacy Filters

Gmail integration respects your privacy through configurable filters:

```yaml
# Advanced privacy configuration
gmail:
  privacy_filters:
    enabled: true

    # Label-based exclusion
    exclude_labels:
      - "Personal"
      - "Banking"
      - "Medical"
      - "Legal"

    # Content pattern exclusion (regex supported)
    exclude_patterns:
      - "SSN: \\d{3}-\\d{2}-\\d{4}"
      - "Credit Card.*\\d{4}"
      - "Password.*:"

    # Sender/domain exclusion
    exclude_domains:
      - "bank.example.com"
      - "medical.provider.com"
    exclude_senders:
      - "sensitive.contact@example.com"

    # Include only specific labels (allowlist mode)
    include_only_labels:
      - "Work"
      - "Projects"
      # Leave empty to disable allowlist mode
```

### 7.2 Credential Security

Your Gmail credentials are protected through:
- **AES-256 encryption** for stored OAuth2 tokens
- **Automatic token refresh** without exposing secrets
- **Secure key management** using system keyring
- **No password storage** - only OAuth2 tokens

To rotate credentials:

```bash
# Revoke current access
penf gmail disconnect --account your.email@gmail.com

# Re-authenticate with fresh tokens
penf gmail connect --account your.email@gmail.com
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
Solution: Wait 24 hours or request quota increase
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
Solution: Increase timeout or exclude large files
Config: attachment_timeout_seconds: 300
```

### Support and Logs

View detailed logs:
```bash
penf gmail logs --tail
penf gmail logs --level debug  # More detailed output
```

Report issues with context:
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
5. **Secure Storage**: Ensure config files have restricted permissions (600)

```bash
# Secure your configuration
chmod 600 ~/.penfold/config.yaml
chmod 600 ~/.penfold/gmail_credentials.json
```