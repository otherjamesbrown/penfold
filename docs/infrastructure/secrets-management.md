# Secrets Management

This document provides comprehensive guidance for secure handling of secrets and sensitive credentials in the Penfold project.

## Table of Contents

- [Overview](#overview)
- [Secrets Storage Strategy](#secrets-storage-strategy)
- [Secrets Inventory](#secrets-inventory)
- [API Key Management](#api-key-management)
- [Database Credential Management](#database-credential-management)
- [SSL Certificate Management](#ssl-certificate-management)
- [Development vs Production](#development-vs-production)
- [CI/CD Integration](#cicd-integration)
- [Security Best Practices](#security-best-practices)

---

## Overview

Penfold uses a layered approach to secrets management:

1. **Environment variables** for runtime configuration
2. **File-based secrets** for complex credentials (OAuth tokens, certificates)
3. **Encrypted storage** for sensitive data at rest
4. **External secrets directory** for local development

### Directory Structure

```
~/github/otherjamesbrown/secrets/    # External secrets directory (not committed)
    gemini                           # Gemini API key
    github                           # GitHub PAT
    hugging-face                     # HuggingFace token
    linode                           # Linode credentials
    tokens.md                        # Token documentation

~/.penfold/                          # Application data directory
    encryption-keys/                 # Fernet encryption keys
    gmail_oauth_config.json          # Gmail OAuth2 client configuration
```

---

## Secrets Storage Strategy

### Environment Variables

Environment variables are the primary mechanism for runtime secrets. All configuration is loaded via Pydantic settings classes.

| Strategy | Use Case | Example |
|----------|----------|---------|
| Environment variables | Simple values, API keys | `OPENAI_API_KEY`, `DB_PASSWORD` |
| File paths | Complex credentials, certificates | `ENCRYPTION_KEY_PATH`, `GOOGLE_CLOUD_CREDENTIALS_PATH` |
| Encrypted storage | Sensitive data at rest | Gmail OAuth tokens via `CredentialEncryption` |

### Configuration Loading

```python
# From penf_lib/storage/config.py
class DatabaseConfig(BaseSettings):
    host: str = Field(default="localhost", env="DB_HOST")
    password: str = Field(default="", env="DB_PASSWORD")

    class Config:
        env_prefix = ""
        case_sensitive = False
```

---

## Secrets Inventory

### Database Credentials

| Variable | Description | Required | Default |
|----------|-------------|----------|---------|
| `DB_HOST` | PostgreSQL host | No | `localhost` |
| `DB_PORT` | PostgreSQL port | No | `5432` |
| `DB_NAME` | Database name | No | `penfold_dev` |
| `DB_USER` | Database user | No | Current Unix user |
| `DB_PASSWORD` | Database password | **Production: Yes** | Empty |
| `DB_POOL_SIZE` | Connection pool size | No | `10` |
| `DB_MAX_OVERFLOW` | Max overflow connections | No | `5` |
| `PENF_DATABASE_URL` | Full connection URL (alternative) | No | See below |

**Full URL format:**
```
postgresql+asyncpg://user:password@host:port/database
```

### Redis Credentials

| Variable | Description | Required | Default |
|----------|-------------|----------|---------|
| `REDIS_HOST` | Redis host | No | `localhost` |
| `REDIS_PORT` | Redis port | No | `6379` |
| `REDIS_DB` | Redis database number | No | `0` |
| `REDIS_PASSWORD` | Redis password | **Production: Yes** | None |
| `REDIS_URL` | Full connection URL (alternative) | No | - |

### Gmail OAuth Credentials

Gmail integration requires OAuth2 credentials from the Google Cloud Console.

| File/Variable | Description | Required |
|---------------|-------------|----------|
| `gmail_oauth_config.json` | OAuth2 client configuration | Yes for Gmail |
| Encrypted tokens (DB) | Access/refresh tokens | Auto-managed |

**OAuth Configuration Structure:**
```json
{
  "installed": {
    "client_id": "YOUR_CLIENT_ID.apps.googleusercontent.com",
    "client_secret": "YOUR_CLIENT_SECRET",
    "auth_uri": "https://accounts.google.com/o/oauth2/auth",
    "token_uri": "https://oauth2.googleapis.com/token",
    "redirect_uris": ["http://localhost:8080/oauth/callback"]
  }
}
```

**Storage location:** `~/.penfold/gmail_oauth_config.json`

### AI/ML API Keys

| Variable | Description | Required | Provider |
|----------|-------------|----------|----------|
| `OPENAI_API_KEY` | OpenAI API key | Conditional | OpenAI |
| `GEMINI_API_KEY` | Google Gemini API key | Conditional | Google |
| `GOOGLE_CLOUD_CREDENTIALS_PATH` | GCP service account path | Conditional | Google Cloud |

**Note:** At least one AI provider key is required for embeddings and AI coordination features.

### Encryption and Security

| Variable | Description | Required | Default |
|----------|-------------|----------|---------|
| `PENF_MASTER_KEY` | Master key for credential encryption | **Production: Yes** | `default-dev-key` |
| `JWT_SECRET_KEY` | JWT signing key | **Yes** | None (required) |
| `ENCRYPTION_KEY_PATH` | Path to encryption keys | No | `/var/lib/penfold/encryption-keys` |

**JWT Secret Requirements:**
- Minimum 32 characters
- Cannot use default development values
- Must be unique per environment

### Application Settings

| Variable | Description | Required | Default |
|----------|-------------|----------|---------|
| `PENFOLD_ENV` | Environment name | No | `development` |
| `PENFOLD_TENANT_ID` | Default tenant ID | No | `default` |
| `PENF_DB_ECHO` | SQL query logging | No | `false` |
| `PENF_DB_PERFORMANCE_LEVEL` | DB optimization level | No | `balanced` |

---

## API Key Management

### Generating New Keys

**OpenAI:**
1. Visit https://platform.openai.com/api-keys
2. Create new secret key with appropriate permissions
3. Store in `~/github/otherjamesbrown/secrets/openai` or environment

**Gemini:**
1. Visit https://makersuite.google.com/app/apikey
2. Create API key for your project
3. Store in `~/github/otherjamesbrown/secrets/gemini`

**Google Cloud (Service Account):**
1. Go to GCP Console > IAM & Admin > Service Accounts
2. Create service account with required roles
3. Download JSON key file
4. Set `GOOGLE_CLOUD_CREDENTIALS_PATH` to file location

### Rotation Procedures

**API Keys:**
1. Generate new key in provider console
2. Update environment/secrets with new key
3. Verify application works with new key
4. Revoke old key in provider console

**Rotation Schedule:**
- API keys: Every 90 days or after suspected compromise
- JWT secret: Every 180 days or after security incident
- Database passwords: Every 90 days in production

---

## Database Credential Management

### Local Development

For local development, use peer authentication (no password):

```bash
# In postgresql.conf or pg_hba.conf
local   penfold_dev     james           peer
```

Or create a dedicated development user:

```sql
CREATE USER penfold_dev WITH PASSWORD 'dev-password';
CREATE DATABASE penfold_dev OWNER penfold_dev;
```

### Production Setup

1. Use strong, randomly generated passwords:
   ```bash
   openssl rand -base64 32
   ```

2. Configure environment:
   ```bash
   export DB_HOST=your-db-host
   export DB_USER=penfold_prod
   export DB_PASSWORD=$(cat /run/secrets/db_password)
   export DB_NAME=penfold_prod
   ```

3. Enable SSL for database connections (see SSL section)

### Password Rotation

```bash
# 1. Generate new password
NEW_PASS=$(openssl rand -base64 32)

# 2. Update in PostgreSQL
psql -c "ALTER USER penfold_prod WITH PASSWORD '$NEW_PASS';"

# 3. Update application secrets
# 4. Restart application to pick up new credentials
# 5. Verify connectivity
```

---

## SSL Certificate Management

### Database SSL

For production PostgreSQL connections:

```bash
# Required files
/etc/ssl/certs/db-client.crt    # Client certificate
/etc/ssl/private/db-client.key  # Client key
/etc/ssl/certs/db-ca.crt        # CA certificate

# Connection string with SSL
postgresql+asyncpg://user:pass@host:5432/db?ssl=require&sslrootcert=/etc/ssl/certs/db-ca.crt
```

### Redis SSL/TLS

```bash
# Connection URL with TLS
rediss://user:pass@host:6379/0
```

### Certificate Renewal

1. Generate new certificates before expiration (30 days prior)
2. Deploy new certificates alongside existing
3. Update application configuration
4. Remove old certificates after grace period

---

## Development vs Production

### Development Environment

Create `.env` from template:

```bash
cp .env.example .env
```

**Development Defaults:**
```bash
# .env for local development
PENFOLD_ENV=development
DB_HOST=localhost
DB_NAME=penfold_dev
# DB_PASSWORD not required with peer auth
PENF_MASTER_KEY=dev-master-key-not-for-production
JWT_SECRET_KEY=dev-jwt-secret-key-minimum-32-chars
DEBUG=true
```

**Loading secrets from external directory:**
```bash
# In shell profile or before running
export GEMINI_API_KEY=$(cat ~/github/otherjamesbrown/secrets/gemini)
export OPENAI_API_KEY=$(cat ~/github/otherjamesbrown/secrets/openai 2>/dev/null)
```

### Production Environment

**Required secrets for production:**
- `DB_PASSWORD` - Strong database password
- `REDIS_PASSWORD` - Redis authentication
- `JWT_SECRET_KEY` - Secure JWT signing key
- `PENF_MASTER_KEY` - Strong encryption master key
- At least one AI provider key

**Production Configuration:**
```bash
PENFOLD_ENV=production
DB_HOST=prod-db.example.com
DB_PASSWORD=${DB_PASSWORD}  # From secrets manager
JWT_SECRET_KEY=${JWT_SECRET_KEY}  # From secrets manager
PENF_MASTER_KEY=${PENF_MASTER_KEY}  # From secrets manager
DEBUG=false
```

### Environment Validation

The application validates critical settings at startup:

```python
# From app/config.py
@validator("JWT_SECRET_KEY")
def validate_jwt_secret_key(cls, v):
    if not v:
        raise ValueError("JWT_SECRET_KEY is required")
    if len(v) < 32:
        raise ValueError("JWT_SECRET_KEY must be at least 32 characters")
    if v in ["dev-secret-key", "change-me"]:
        raise ValueError("Cannot use default development values in production")
```

---

## CI/CD Integration

### GitHub Actions Secrets

Configure these secrets in GitHub repository settings:

| Secret Name | Description |
|-------------|-------------|
| `DATABASE_URL` | Test database connection string |
| `REDIS_URL` | Test Redis connection string |
| `JWT_SECRET_KEY` | JWT secret for testing |
| `PENF_MASTER_KEY` | Encryption key for testing |
| `OPENAI_API_KEY` | (Optional) For integration tests |
| `GEMINI_API_KEY` | (Optional) For integration tests |

### Workflow Configuration

```yaml
# .github/workflows/test.yml
name: Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest

    services:
      postgres:
        image: postgres:16
        env:
          POSTGRES_USER: test
          POSTGRES_PASSWORD: test
          POSTGRES_DB: penfold_test
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
        ports:
          - 5432:5432

      redis:
        image: redis:7
        ports:
          - 6379:6379

    env:
      PENFOLD_ENV: testing
      DB_HOST: localhost
      DB_USER: test
      DB_PASSWORD: test
      DB_NAME: penfold_test
      REDIS_URL: redis://localhost:6379/0
      JWT_SECRET_KEY: ${{ secrets.JWT_SECRET_KEY }}
      PENF_MASTER_KEY: ${{ secrets.PENF_MASTER_KEY }}

    steps:
      - uses: actions/checkout@v4

      - name: Set up Python
        uses: actions/setup-python@v5
        with:
          python-version: '3.12'

      - name: Run tests
        run: pytest
```

### Secret Rotation in CI/CD

When rotating secrets:
1. Update secret in GitHub repository settings
2. Re-run affected workflows to verify
3. Document rotation date

---

## Security Best Practices

### General Guidelines

1. **Never commit secrets** - Use `.gitignore` for `.env` files
2. **Use environment variables** - Not hardcoded values
3. **Encrypt at rest** - Use `CredentialEncryption` for sensitive data
4. **Least privilege** - Grant minimal permissions needed
5. **Audit access** - Log credential usage

### Credential Encryption

Penfold uses Fernet symmetric encryption for sensitive data:

```python
from penf_lib.storage.encryption import CredentialEncryption

# Initialize with master key from environment
encryption = CredentialEncryption()

# Encrypt sensitive data
encrypted = await encryption.encrypt({"token": "secret_value"})

# Decrypt when needed
decrypted = await encryption.decrypt(encrypted)
```

**Key derivation:** PBKDF2-HMAC-SHA256 with 100,000 iterations

**Dynamic salt:** A unique 16-byte salt is generated on first use and stored at `~/.penfold/encryption_salt` (configurable via `PENF_SALT_PATH` environment variable). This ensures each installation has unique encryption keys even with the same master password.

**Production key validation:** In production environments (`config.is_production=True`), the encryption module will raise a `ValueError` if the default development key is used. You must set the `PENF_MASTER_KEY` environment variable to a secure random string.

### Gmail Credentials Protection

OAuth tokens are automatically encrypted before storage:

```python
# From penf_lib/connectors/gmail/auth.py
async def encrypt_credentials(self, credentials: Credentials) -> bytes:
    credential_data = {
        'token': credentials.token,
        'refresh_token': credentials.refresh_token,
        'client_id': credentials.client_id,
        'client_secret': credentials.client_secret,
        # ...
    }
    return await self._encryption.encrypt(credential_data)
```

### Monitoring and Alerting

Set up alerts for:
- Failed authentication attempts
- Unusual API usage patterns
- Certificate expiration (30 days warning)
- Secret access anomalies

### Incident Response

If credentials are compromised:

1. **Immediately revoke** the compromised credential
2. **Generate new** credentials
3. **Update** all systems using the credential
4. **Audit** access logs for unauthorized use
5. **Document** the incident and response

### Checklist for New Deployments

- [ ] All required secrets configured
- [ ] No default/development values in production
- [ ] SSL/TLS enabled for database and Redis
- [ ] JWT secret is unique and strong
- [ ] Master encryption key is unique and backed up securely
- [ ] API keys have appropriate scope/permissions
- [ ] Secrets are not logged or exposed in error messages
- [ ] Rotation schedule documented and followed

---

## Quick Reference

### Loading Secrets (Development)

```bash
# Load from secrets directory
source <(cat <<EOF
export GEMINI_API_KEY=$(cat ~/github/otherjamesbrown/secrets/gemini)
export GITHUB_TOKEN=$(cat ~/github/otherjamesbrown/secrets/github)
EOF
)
```

### Environment File Template

```bash
# Copy and customize
cp .env.example .env
vim .env
```

### Key Generation Commands

```bash
# Generate secure password
openssl rand -base64 32

# Generate JWT secret
python -c "import secrets; print(secrets.token_urlsafe(48))"

# Generate Fernet key
python -c "from cryptography.fernet import Fernet; print(Fernet.generate_key().decode())"
```

### Validation Commands

```bash
# Check environment configuration
python -c "from penf_lib.storage.config import config; print(config.database.database_url)"

# Test database connection
python -c "
import asyncio
from penf_lib.storage.database import db_manager
async def test():
    async with db_manager.get_session() as session:
        print('Connection successful')
asyncio.run(test())
"
```

---

## Related Documentation

- [Gmail Integration Architecture](../gmail-integration/architecture.md)
- [Database Schema](../database-schema/)
- [Testing Framework](../testing-framework/)
