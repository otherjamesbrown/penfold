# Gmail Integration Troubleshooting Guide

This guide helps diagnose and resolve common issues with Gmail integration in Penfold.

## Table of Contents

- [Quick Diagnostic Commands](#quick-diagnostic-commands)
- [Authentication Issues](#authentication-issues)
- [Synchronization Problems](#synchronization-problems)
- [Rate Limiting and API Quota](#rate-limiting-and-api-quota)
- [Real-time Monitoring Issues](#real-time-monitoring-issues)
- [Attachment Processing Problems](#attachment-processing-problems)
- [Privacy Filter Issues](#privacy-filter-issues)
- [Performance Problems](#performance-problems)
- [Multi-Account Issues](#multi-account-issues)
- [Database and Storage Issues](#database-and-storage-issues)
- [Network and Connectivity](#network-and-connectivity)
- [Logging and Diagnostics](#logging-and-diagnostics)
- [Recovery Procedures](#recovery-procedures)

## Quick Diagnostic Commands

Start troubleshooting with these diagnostic commands:

```bash
# Check overall Gmail integration status
penf gmail status --verbose

# Run comprehensive health check
penf gmail healthcheck

# Test API connectivity
penf gmail test-connection

# Generate diagnostic report
penf gmail diagnostic --export /tmp/gmail-diagnostic.zip

# Check recent errors
penf gmail logs --level error --tail 50
```

## Authentication Issues

### Problem: OAuth2 Authentication Fails

**Symptoms:**
- Browser redirects to error page during OAuth2 flow
- "Invalid client" or "unauthorized client" errors
- Authentication callback never completes

**Diagnostic Commands:**
```bash
penf gmail status --account user@gmail.com
penf gmail logs --grep "OAuth2" --level debug
```

**Common Causes & Solutions:**

#### 1. Invalid OAuth2 Credentials

```bash
# Check credentials file
penf gmail config --check-credentials

# Expected output should show valid client_id and client_secret
```

**Solution:**
1. Verify `gmail_credentials.json` contains correct OAuth2 credentials
2. Ensure credentials are for "Desktop Application" type, not "Web Application"
3. Re-download credentials from Google Cloud Console if corrupted

#### 2. Incorrect OAuth2 Scopes

**Error:** `Error 400: invalid_scope`

**Solution:**
```bash
# Check configured scopes
penf gmail config --list-scopes

# Reset to required scopes
penf gmail config --scopes "https://www.googleapis.com/auth/gmail.readonly,https://www.googleapis.com/auth/gmail.labels"
```

#### 3. OAuth2 Consent Screen Issues

**Error:** `Error 403: access_denied`

**Solution:**
1. Ensure OAuth2 consent screen is configured in Google Cloud Console
2. Add your email to test users if app is in testing mode
3. Verify app is published if using external user type

#### 4. Expired or Revoked Tokens

**Symptoms:**
- Previously working authentication suddenly fails
- "invalid_grant" errors during token refresh

**Diagnostic:**
```bash
penf gmail auth-status --account user@gmail.com
```

**Solution:**
```bash
# Refresh authentication
penf gmail refresh-auth --account user@gmail.com

# If refresh fails, re-authenticate
penf gmail disconnect --account user@gmail.com
penf gmail connect --account user@gmail.com
```

### Problem: Credential Encryption Errors

**Symptoms:**
- "Decryption failed" errors in logs
- Unable to access stored OAuth2 tokens

**Diagnostic Commands:**
```bash
penf gmail config --check-encryption
penf gmail logs --grep "encryption" --level error
```

**Solution:**
```bash
# Reset encryption (will require re-authentication)
penf gmail reset-encryption
penf gmail connect --account user@gmail.com
```

## Synchronization Problems

### Problem: Historical Import Stalls or Fails

**Symptoms:**
- Import progress stops advancing
- "Sync operation failed" errors
- Partial email imports

**Diagnostic Commands:**
```bash
penf gmail import --status
penf gmail sync --dry-run --account user@gmail.com
penf gmail logs --grep "import" --level info
```

**Common Causes & Solutions:**

#### 1. Large Email Volume

**Symptoms:**
- Import takes extremely long time
- Memory usage increases dramatically
- Timeouts during processing

**Solution:**
```bash
# Reduce batch size
penf gmail config --batch-size 25

# Limit import scope
penf gmail import --from-date 2025-01-01 --max-messages 1000

# Import in smaller chunks
penf gmail import --from-date 2024-01-01 --to-date 2024-06-30
penf gmail import --from-date 2024-07-01 --to-date 2024-12-31
```

#### 2. Rate Limiting During Import

**Symptoms:**
- Import speed decreases significantly
- "Rate limit exceeded" warnings in logs

**Solution:**
```bash
# Check current rate limiting status
penf gmail quota-status

# Adjust rate limiting parameters
penf gmail config --rate-limit-requests 100  # Reduce from default 250
penf gmail config --sync-interval 120        # Increase interval

# Resume import after quota reset
penf gmail import --resume
```

#### 3. Network Timeouts

**Symptoms:**
- "Request timeout" errors
- Intermittent connection failures

**Solution:**
```bash
# Increase timeout values
penf gmail config --request-timeout 60       # Increase from 30 seconds
penf gmail config --retry-attempts 5         # Increase retries

# Test network connectivity
penf gmail test-connection --verbose
```

### Problem: Incremental Sync Missing Emails

**Symptoms:**
- New emails not appearing in Penfold
- Sync status shows "up to date" but emails are missing

**Diagnostic Commands:**
```bash
penf gmail sync-status --account user@gmail.com
penf gmail test-email --account user@gmail.com  # Send test email
penf gmail logs --grep "incremental" --level debug
```

**Solutions:**

#### 1. Reset Sync State

```bash
# Clear incremental sync state and start fresh
penf gmail reset-sync --account user@gmail.com
penf gmail sync --full --account user@gmail.com
```

#### 2. Check Gmail History ID

```bash
# Compare local and Gmail history IDs
penf gmail debug-history --account user@gmail.com

# If out of sync, reset and resync
penf gmail reset-history --account user@gmail.com
```

## Rate Limiting and API Quota

### Problem: Gmail API Quota Exceeded

**Symptoms:**
- "Quota exceeded" errors in logs
- Sync operations failing with 429 HTTP status
- Unable to fetch new emails

**Diagnostic Commands:**
```bash
penf gmail quota-status
penf gmail logs --grep "quota\|429" --level warn
```

**Solutions:**

#### 1. Check Daily Quota Usage

```bash
penf gmail quota-status --detailed

# Example output:
# Daily Quota: 1,000,000,000 units
# Used Today: 856,234,192 units (85.6%)
# Rate Limit: 250 units/second
# Current Rate: 180 units/second
```

#### 2. Optimize Request Patterns

```bash
# Reduce batch sizes
penf gmail config --batch-size 50

# Increase sync intervals
penf gmail config --sync-interval 300  # 5 minutes instead of 1 minute

# Prioritize accounts
penf gmail config --account work@company.com --priority high
penf gmail config --account personal@gmail.com --priority low
```

#### 3. Request Quota Increase

If consistently hitting quota limits:

1. Go to Google Cloud Console > APIs & Services > Quotas
2. Find "Gmail API" quotas
3. Request increase with business justification
4. Typical approval takes 2-3 business days

#### 4. Implement Smart Quota Management

```bash
# Enable intelligent quota distribution
penf gmail config --smart-quota true

# Set quota allocation per account
penf gmail config --account user@gmail.com --quota-percent 60
penf gmail config --account personal@gmail.com --quota-percent 40
```

### Problem: Per-User Rate Limiting

**Symptoms:**
- Consistent 429 errors for specific accounts
- Rate limiting even with low overall quota usage

**Solution:**
```bash
# Check per-user rate limiting
penf gmail rate-status --account user@gmail.com

# Reduce request rate for specific account
penf gmail config --account user@gmail.com --max-requests-per-second 100

# Implement account-specific backoff
penf gmail config --account user@gmail.com --backoff-multiplier 2.0
```

## Real-time Monitoring Issues

### Problem: Push Notifications Not Working

**Symptoms:**
- New emails detected only during manual sync
- Real-time latency exceeding target (>60 seconds)
- No webhook notifications in logs

**Diagnostic Commands:**
```bash
penf gmail monitor-status
penf gmail test-webhook
penf gmail logs --grep "webhook\|push" --level debug
```

**Common Causes & Solutions:**

#### 1. Cloud Pub/Sub Configuration

**Check Configuration:**
```bash
penf gmail config --list-pubsub

# Verify settings:
# - pubsub_topic: projects/your-project/topics/gmail-notifications
# - subscription: projects/your-project/subscriptions/gmail-penfold
# - webhook_url: https://your-domain.com/webhooks/gmail
```

**Fix Configuration:**
```bash
# Reconfigure Pub/Sub settings
penf gmail setup-pubsub --project your-project-id
penf gmail setup-webhook --url https://your-domain.com/webhooks/gmail
```

#### 2. Webhook Endpoint Issues

**Test Webhook Accessibility:**
```bash
# Test webhook endpoint
curl -X POST https://your-domain.com/webhooks/gmail \
  -H "Content-Type: application/json" \
  -d '{"test": true}'

# Should return 200 OK
```

**Common Issues:**
- Webhook URL not publicly accessible
- SSL certificate problems
- Firewall blocking webhook port
- Incorrect Content-Type handling

#### 3. Subscription Configuration

**Check Subscription Status:**
```bash
penf gmail pubsub-status

# Expected output:
# Subscription: projects/your-project/subscriptions/gmail-penfold
# Status: ACTIVE
# Undelivered Messages: 0
# Push Config: https://your-domain.com/webhooks/gmail
```

**Reset Subscription:**
```bash
# Delete and recreate subscription
penf gmail reset-pubsub --account user@gmail.com
```

### Problem: Polling Fallback Not Working

**Symptoms:**
- No email detection when Push notifications fail
- Polling intervals not being respected

**Solution:**
```bash
# Enable and configure polling fallback
penf gmail config --polling-fallback true
penf gmail config --polling-interval 300  # 5 minutes

# Check polling status
penf gmail monitor-status --polling
```

## Attachment Processing Problems

### Problem: Attachments Not Processing

**Symptoms:**
- Emails imported but attachments missing
- "Attachment processing failed" errors
- Deferred processing never completes

**Diagnostic Commands:**
```bash
penf gmail attachment-status
penf gmail logs --grep "attachment" --level warn
penf gmail queue-status  # Background processing queue
```

**Common Causes & Solutions:**

#### 1. Attachment Size Limits

**Check Size Limits:**
```bash
penf gmail config --list-attachment-limits

# Default limits:
# max_size_mb: 10
# supported_formats: pdf,docx,txt,jpeg,png
```

**Adjust Limits:**
```bash
# Increase size limit (be careful with memory usage)
penf gmail config --max-attachment-size 25

# Add support for additional formats
penf gmail config --supported-formats "pdf,docx,txt,jpeg,png,xlsx,pptx"
```

#### 2. Content Extraction Failures

**Check Extraction Libraries:**
```bash
# Verify required libraries are installed
pip list | grep -E "PyPDF2|python-docx|Pillow|tesseract"
```

**Install Missing Dependencies:**
```bash
# Install optional dependencies for attachment processing
pip install penfold[attachments]

# Or install individually
pip install PyPDF2 python-docx Pillow pytesseract
```

#### 3. Background Queue Issues

**Check Queue Status:**
```bash
penf gmail queue-status

# Example output:
# Pending Tasks: 23
# Active Workers: 2
# Failed Tasks: 1
# Queue Health: OK
```

**Fix Queue Problems:**
```bash
# Restart queue workers
penf gmail restart-workers

# Clear failed tasks (after investigating)
penf gmail clear-failed-tasks

# Increase worker count
penf gmail config --attachment-workers 4
```

### Problem: OCR (Image Text Extraction) Failing

**Symptoms:**
- Image attachments processed but no text extracted
- "Tesseract not found" errors

**Solution:**
```bash
# Install Tesseract OCR engine
# macOS:
brew install tesseract

# Ubuntu/Debian:
sudo apt-get install tesseract-ocr

# Verify installation
tesseract --version

# Configure Tesseract path if needed
penf gmail config --tesseract-path /usr/local/bin/tesseract
```

## Privacy Filter Issues

### Problem: Legitimate Emails Being Filtered

**Symptoms:**
- Important emails missing from processing
- Unexpected privacy filter matches

**Diagnostic Commands:**
```bash
penf gmail privacy-audit --account user@gmail.com
penf gmail logs --grep "privacy\|filtered" --level info
```

**Solutions:**

#### 1. Review Filter Configuration

```bash
# List current privacy filters
penf gmail config --list-privacy-filters

# Check which emails were filtered and why
penf gmail privacy-audit --show-filtered --last 7d
```

#### 2. Adjust Filter Sensitivity

```bash
# Disable specific filter types temporarily
penf gmail config --disable-pattern-filters

# Add exceptions for specific senders/domains
penf gmail config --add-trusted-sender important@company.com
penf gmail config --add-trusted-domain company.com

# Adjust regex patterns
penf gmail config --exclude-patterns "password\\s+reset,unsubscribe"
```

#### 3. Test Filter Rules

```bash
# Test filter against specific email
penf gmail test-privacy-filter --message-id abc123

# Simulate filter against sample text
penf gmail test-privacy-filter --text "This contains sensitive SSN: 123-45-6789"
```

### Problem: Privacy Filters Not Applying

**Symptoms:**
- Sensitive content appearing in processed emails
- Privacy audit shows no filtering activity

**Solution:**
```bash
# Verify privacy filters are enabled
penf gmail config --privacy-enabled true

# Check filter configuration syntax
penf gmail validate-privacy-config

# Test filter patterns
penf gmail test-privacy-filters --verbose
```

## Performance Problems

### Problem: Slow Email Processing

**Symptoms:**
- Historical import taking much longer than expected
- Real-time sync latency consistently high
- High CPU or memory usage

**Diagnostic Commands:**
```bash
penf gmail performance-stats
penf gmail logs --grep "performance\|slow" --level warn
top -p $(pgrep -f "penf gmail")  # Monitor resource usage
```

**Solutions:**

#### 1. Optimize Batch Processing

```bash
# Reduce batch size for memory-constrained environments
penf gmail config --batch-size 25

# Increase for high-memory environments
penf gmail config --batch-size 200

# Enable parallel processing
penf gmail config --parallel-processing true
penf gmail config --max-workers 4
```

#### 2. Database Performance

```bash
# Check database performance
penf gmail db-stats

# Optimize database indices
penf gmail optimize-db

# Consider database tuning parameters in PostgreSQL:
# shared_buffers = 256MB
# effective_cache_size = 1GB
# random_page_cost = 1.1  # For SSD storage
```

#### 3. Network Optimization

```bash
# Enable connection pooling
penf gmail config --connection-pooling true
penf gmail config --pool-size 10

# Optimize request concurrency
penf gmail config --max-concurrent-requests 8
```

### Problem: High Memory Usage

**Symptoms:**
- Python process consuming excessive RAM
- Out of memory errors during processing
- System becoming unresponsive

**Solution:**
```bash
# Reduce memory footprint
penf gmail config --batch-size 10          # Smaller batches
penf gmail config --disable-content-cache  # Don't cache email content
penf gmail config --stream-attachments true # Stream large attachments

# Monitor memory usage
penf gmail memory-stats --monitor
```

## Multi-Account Issues

### Problem: Account Priority Not Working

**Symptoms:**
- Low-priority accounts syncing more frequently than high-priority
- Uneven resource allocation across accounts

**Diagnostic Commands:**
```bash
penf gmail accounts --show-priority
penf gmail sync-schedule --show-all
```

**Solution:**
```bash
# Reset account priorities
penf gmail config --account work@company.com --priority high --sync-interval 60
penf gmail config --account personal@gmail.com --priority medium --sync-interval 300
penf gmail config --account archive@gmail.com --priority low --sync-interval 1800

# Verify priority scheduling
penf gmail sync-schedule --validate
```

### Problem: Cross-Account Quota Conflicts

**Symptoms:**
- Some accounts unable to sync due to quota exhaustion by others
- Uneven quota distribution

**Solution:**
```bash
# Enable quota management
penf gmail config --enable-quota-management true

# Set per-account quota allocations
penf gmail config --account work@company.com --quota-percent 50
penf gmail config --account personal@gmail.com --quota-percent 30
penf gmail config --account archive@gmail.com --quota-percent 20

# Monitor quota usage by account
penf gmail quota-usage --by-account
```

## Database and Storage Issues

### Problem: Database Connection Failures

**Symptoms:**
- "Unable to connect to database" errors
- Sync operations timing out
- Data consistency issues

**Diagnostic Commands:**
```bash
penf gmail db-status
penf gmail test-db-connection
penf gmail logs --grep "database\|postgresql" --level error
```

**Solutions:**

#### 1. Connection Pool Issues

```bash
# Check connection pool status
penf gmail db-pool-status

# Restart connection pool
penf gmail restart-db-pool

# Adjust pool settings
penf gmail config --db-pool-size 20
penf gmail config --db-max-overflow 30
```

#### 2. Database Deadlocks

**Symptoms:**
- "Deadlock detected" errors in logs
- Concurrent sync operations failing

**Solution:**
```bash
# Reduce concurrency to avoid deadlocks
penf gmail config --max-concurrent-syncs 2

# Enable deadlock retry logic
penf gmail config --deadlock-retry true
penf gmail config --deadlock-max-retries 3
```

### Problem: Storage Space Issues

**Symptoms:**
- "Disk full" errors during attachment processing
- Unable to store new email data

**Solution:**
```bash
# Check storage usage
penf gmail storage-stats

# Clean up old attachments
penf gmail cleanup-attachments --older-than 90d

# Configure automatic cleanup
penf gmail config --auto-cleanup true
penf gmail config --retention-days 180

# Move attachment storage to different location
penf gmail config --attachment-storage-path /path/to/larger/disk
```

## Network and Connectivity

### Problem: Intermittent Connection Failures

**Symptoms:**
- Random "Connection reset by peer" errors
- Timeouts during Gmail API requests
- Inconsistent sync behavior

**Diagnostic Commands:**
```bash
penf gmail network-test --continuous
penf gmail logs --grep "connection\|timeout\|network" --level warn
```

**Solutions:**

#### 1. Network Stability Issues

```bash
# Increase timeout values for unreliable connections
penf gmail config --request-timeout 60
penf gmail config --connect-timeout 30

# Enable aggressive retry logic
penf gmail config --retry-attempts 5
penf gmail config --retry-backoff-multiplier 2.0
```

#### 2. DNS Resolution Problems

```bash
# Test DNS resolution for Gmail API
nslookup gmail.googleapis.com

# Use alternative DNS servers if needed
penf gmail config --dns-servers "8.8.8.8,8.8.4.4"
```

#### 3. Firewall or Proxy Issues

```bash
# Test direct connection to Gmail API
curl -I https://gmail.googleapis.com/

# Configure proxy if needed
penf gmail config --proxy-url http://proxy.company.com:8080
penf gmail config --proxy-auth username:password
```

## Logging and Diagnostics

### Comprehensive Diagnostic Collection

When reporting issues, collect comprehensive diagnostic information:

```bash
# Generate complete diagnostic report
penf gmail diagnostic --full --export /tmp/gmail-diagnostic-$(date +%Y%m%d).zip

# The report includes:
# - Configuration files (credentials redacted)
# - Recent log files
# - Database schema and connection info
# - System resource usage
# - Network connectivity tests
# - API quota status
# - Account status for all connected accounts
```

### Log Level Configuration

```bash
# Enable debug logging for troubleshooting
penf gmail config --log-level debug

# Filter logs by component
penf gmail logs --component auth --level error
penf gmail logs --component sync --level info
penf gmail logs --component privacy --level debug

# Real-time log monitoring
penf gmail logs --tail --follow
```

### Custom Diagnostic Scripts

```bash
# Create custom health check script
cat > ~/.penfold/health-check.sh << 'EOF'
#!/bin/bash
echo "=== Gmail Integration Health Check ==="
penf gmail status
penf gmail test-connection
penf gmail quota-status
penf gmail db-status
penf gmail monitor-status
echo "=== Health Check Complete ==="
EOF

chmod +x ~/.penfold/health-check.sh
~/.penfold/health-check.sh
```

## Recovery Procedures

### Complete Gmail Integration Reset

If all else fails, reset the entire Gmail integration:

```bash
# WARNING: This will require re-authentication and full re-sync

# 1. Stop all Gmail services
penf gmail stop-all

# 2. Backup current configuration
cp ~/.penfold/config.yaml ~/.penfold/config.yaml.backup

# 3. Clear all Gmail data (DESTRUCTIVE)
penf gmail reset --confirm --all-data

# 4. Remove authentication credentials
penf gmail disconnect --all-accounts

# 5. Reset database tables (if needed)
penf gmail reset-db --tables gmail_connections,sync_operations,sync_state

# 6. Restart with fresh configuration
penf gmail setup --interactive
```

### Partial Recovery - Single Account

To reset just one problematic account:

```bash
# Stop sync for specific account
penf gmail stop-sync --account user@gmail.com

# Disconnect and reconnect account
penf gmail disconnect --account user@gmail.com
penf gmail connect --account user@gmail.com

# Clear sync state for account
penf gmail reset-sync --account user@gmail.com

# Restart sync with fresh state
penf gmail sync --account user@gmail.com --full
```

### Emergency Procedures

#### Stop All Processing Immediately

```bash
# Emergency stop - kills all Gmail processes
penf gmail emergency-stop

# Check if all processes stopped
ps aux | grep -E "penf.*gmail|gmail.*penf"
```

#### Prevent Data Loss During Issues

```bash
# Enable backup mode (read-only)
penf gmail config --read-only-mode true

# Export all data before major changes
penf gmail export --all-accounts --output /backup/gmail-export-$(date +%Y%m%d).json
```

## Getting Additional Help

### Support Channels

1. **Check Documentation**: Review [setup guide](./setup-guide.md) and [architecture docs](./architecture.md)
2. **Search Issues**: Check GitHub issues for similar problems
3. **Community Forum**: Ask questions in the Penfold community forum
4. **Debug Reports**: Always include output from `penf gmail diagnostic` when reporting issues

### Information to Include in Bug Reports

```bash
# Collect this information for bug reports:

# 1. System information
uname -a
python --version
pip list | grep -E "penfold|google|oauth"

# 2. Gmail integration status
penf gmail status --verbose

# 3. Configuration (credentials redacted)
penf gmail config --export --redact-credentials

# 4. Recent error logs
penf gmail logs --level error --last 24h

# 5. Diagnostic report
penf gmail diagnostic --export /tmp/bug-report.zip
```

### Performance Benchmarking

To establish baseline performance metrics:

```bash
# Run performance benchmark
penf gmail benchmark --account user@gmail.com

# Expected results:
# OAuth2 authentication: < 2 minutes
# Historical import: 100+ emails/minute
# Real-time detection: < 60 seconds
# Attachment processing: 90% success rate

# Compare with your results to identify performance issues
```

This troubleshooting guide covers the most common issues encountered with Gmail integration. For issues not covered here, collect diagnostic information using the provided commands and consult the support channels listed above.