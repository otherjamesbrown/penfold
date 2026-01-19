# Architecture Review: Security & Data Flow

**Review Date**: 2026-01-16
**Reviewer**: Architecture Review Pass 2 - Security Analysis
**System**: Penfold - AI-powered personal information system

---

## Summary

Penfold demonstrates a **local-first security posture** that aligns well with its constitutional privacy principles. The architecture includes encryption-at-rest for credentials, multi-tenant data isolation, and privacy-aware search filtering. However, several areas require attention, particularly around authentication implementation completeness, credential management defaults, and input validation consistency.

**Overall Security Posture**: Moderate - Good foundations with notable gaps requiring remediation before production deployment.

---

## Previous Pass Reference

Pass 1 (Structure) identified several items with security implications:

| Finding | Security Relevance | Status in This Review |
|---------|-------------------|----------------------|
| Duplicate code paths (app/ vs penf_lib/) | Inconsistent security controls | **Confirmed** - Privacy filtering differs between implementations |
| Multi-tenancy via RLS + tenant_id | Data isolation mechanism | **Expanded** - Tenant switching needs access control |
| Monolithic models.py | Not directly security relevant | N/A |

---

## Findings

### Strengths

#### 1. Credential Encryption Architecture (Strong)

The system implements proper credential encryption using industry-standard cryptography:

**Location**: `/Users/james/github/otherjamesbrown/penfold/penf_lib/storage/encryption.py`

```python
# PBKDF2-HMAC-SHA256 with 100,000 iterations
kdf = PBKDF2HMAC(
    algorithm=hashes.SHA256(),
    length=32,
    salt=b'penfold-static-salt',
    iterations=100000,
)
```

- Uses Fernet (AES-128-CBC) for symmetric encryption
- Key derivation from master password with strong iteration count
- Async-compatible encryption interface
- OAuth tokens encrypted before database storage

#### 2. Privacy-Aware Search Filtering (Strong)

**Location**: `/Users/james/github/otherjamesbrown/penfold/app/search/privacy.py`

Comprehensive privacy controls:
- Role-based access matrix (GUEST -> SUPER_ADMIN)
- Privacy level hierarchy (PUBLIC, ORGANIZATION, TEAM_ONLY, CONFIDENTIAL)
- Result sanitization removing sensitive fields based on user role
- Search access audit logging
- Sensitive content detection (SSN, credit cards, API keys, passwords)

```python
# Automatic PII redaction
self.sensitive_patterns = {
    'ssn': re.compile(r'\b\d{3}-\d{2}-\d{4}\b'),
    'credit_card': re.compile(r'\b\d{4}[- ]?\d{4}[- ]?\d{4}[- ]?\d{4}\b'),
    'api_key': re.compile(r'\bapi[_\s]*key\s*:?\s*[a-zA-Z0-9]{20,}\b', re.IGNORECASE)
}
```

#### 3. JWT Secret Validation (Strong)

**Location**: `/Users/james/github/otherjamesbrown/penfold/app/config.py`

Proactive validation prevents weak secrets:

```python
@validator("JWT_SECRET_KEY")
def validate_jwt_secret_key(cls, v):
    if len(v) < 32:
        raise ValueError("JWT_SECRET_KEY must be at least 32 characters")
    if v in ["dev-secret-key", "dev-secret-key-change-in-production", "change-me"]:
        raise ValueError("Cannot use default development values in production")
```

#### 4. Webhook Signature Verification (Strong)

**Location**: `/Users/james/github/otherjamesbrown/penfold/penf_lib/connectors/gmail/webhook.py`

Proper HMAC verification for webhook authenticity:

```python
expected = hmac.new(
    self.webhook_secret.encode(),
    body,
    hashlib.sha256
).hexdigest()
return hmac.compare_digest(signature, expected)  # Timing-safe comparison
```

#### 5. File Upload Validation (Strong)

**Location**: `/Users/james/github/otherjamesbrown/penfold/app/upload/validation.py`

- MIME type detection via `python-magic` (not just extension checking)
- Whitelist-based format validation
- Size limits per content type
- SHA-256 checksum calculation for integrity

#### 6. Comprehensive Secrets Documentation (Strong)

**Location**: `/Users/james/github/otherjamesbrown/penfold/docs/infrastructure/secrets-management.md`

Excellent documentation covering:
- Secrets inventory with required/optional flags
- Rotation procedures
- Development vs production configuration
- Key generation commands
- Incident response procedures

---

### Concerns

#### 1. Mock Authentication in Production Code (Critical)

**Severity**: Critical
**Location**: `/Users/james/github/otherjamesbrown/penfold/app/api/search_routes.py:53-73`

The search API uses mock authentication that grants access based on token presence:

```python
async def get_current_user(
    credentials: HTTPAuthorizationCredentials = Depends(security)
) -> Dict[str, Any]:
    # Mock user for demonstration - in production, decode JWT token
    if not credentials:
        return {
            "user_id": "anonymous",
            "role": UserRole.GUEST,
            "team_ids": set()
        }
    # For demo purposes, extract user info from token
    return {
        "user_id": "demo_user",
        "role": UserRole.MEMBER,  # Default role
        "team_ids": {"team1", "team2"}
    }
```

**Risk**: Any request with an Authorization header receives MEMBER-level access. No JWT validation occurs.

**Recommendation**: Implement proper JWT token validation before any production deployment. Consider using a library like `python-jose` or `PyJWT`.

#### 2. Static Encryption Salt (High)

**Severity**: High
**Location**: `/Users/james/github/otherjamesbrown/penfold/penf_lib/storage/encryption.py:29`

```python
# Use a static salt for now (should be configurable in production)
salt = b'penfold-static-salt'
```

**Risk**: All installations share the same salt, weakening key derivation. Rainbow table attacks become more feasible if the static salt is known.

**Recommendation**: Generate per-installation random salt and store it alongside the encrypted data or in a separate configuration.

#### 3. Default Encryption Key in Development (High)

**Severity**: High
**Location**: `/Users/james/github/otherjamesbrown/penfold/penf_lib/storage/encryption.py:22`

```python
self.password = password or os.getenv('PENF_MASTER_KEY', 'default-dev-key')
```

**Risk**: If `PENF_MASTER_KEY` is not set, the system silently uses a hardcoded default key. Credentials encrypted with this key are trivially recoverable.

**Recommendation**: Require `PENF_MASTER_KEY` in production (similar to JWT_SECRET_KEY validation). Raise an error if unset in production environment.

#### 4. Tenant Switching Without Access Control (High)

**Severity**: High
**Location**: `/Users/james/github/otherjamesbrown/penfold/penf_lib/storage/tenant_manager.py:115-116`

```python
# Check access (for now, allow access to all tenants)
# TODO: Implement proper access control
```

**Risk**: Any authenticated user can switch to any tenant, potentially accessing other users' data.

**Recommendation**: Implement tenant access control before multi-tenant deployment. Maintain a user-tenant membership table.

#### 5. SQL Injection Risk in NOTIFY Channel (Medium)

**Severity**: Medium
**Location**: `/Users/james/github/otherjamesbrown/penfold/penf_lib/storage/events.py:166`

```python
await self.session.execute(
    text(f"NOTIFY {channel}, :payload"),
    {"payload": notification_payload}
)
```

**Risk**: The `channel` parameter is interpolated directly into the SQL string. While the payload is parameterized, a malicious channel name could inject SQL.

**Recommendation**: Validate channel names against a whitelist of allowed patterns (e.g., alphanumeric only) or use a predefined set of channels.

#### 6. Upload Endpoints Lack Authentication (Medium)

**Severity**: Medium
**Location**: `/Users/james/github/otherjamesbrown/penfold/app/api/upload_routes.py`

Upload endpoints do not require authentication:

```python
@router.post("/meetings/upload")
async def initiate_upload(
    upload_request: UploadRequest,
    db: AsyncSession = Depends(get_db)
):  # No authentication dependency
```

**Risk**: Unauthenticated users can upload files, potentially filling storage or uploading malicious content.

**Recommendation**: Add `current_user: Dict[str, Any] = Depends(get_current_user)` dependency to all upload endpoints.

#### 7. Admin Endpoints Vulnerable to Role Bypass (Medium)

**Severity**: Medium
**Location**: `/Users/james/github/otherjamesbrown/penfold/app/api/search_routes.py:452-454`

Admin endpoints check roles, but the mock authentication always returns MEMBER:

```python
if current_user["role"] not in [UserRole.ADMIN, UserRole.SUPER_ADMIN]:
    raise HTTPException(status_code=403, detail="Admin access required")
```

**Risk**: Admin functionality is protected, but only as effective as the authentication mechanism (which is currently mock).

**Recommendation**: Admin protection is good, but meaningless until real authentication is implemented.

#### 8. Error Messages May Leak Information (Low)

**Severity**: Low
**Location**: Multiple locations including `/Users/james/github/otherjamesbrown/penfold/app/main.py:86-96`

```python
@app.exception_handler(Exception)
async def global_exception_handler(request, exc):
    return JSONResponse(
        status_code=500,
        content={
            "type": type(exc).__name__  # Leaks exception class names
        }
    )
```

**Risk**: Exception type names can reveal implementation details to attackers.

**Recommendation**: In production, return generic error messages. Log detailed errors server-side only.

#### 9. Subprocess Execution Without Shell Escaping (Low)

**Severity**: Low
**Location**: `/Users/james/github/otherjamesbrown/penfold/app/upload/validation.py:164`

```python
process = await asyncio.create_subprocess_exec(
    *cmd,
    stdout=asyncio.subprocess.PIPE,
    stderr=asyncio.subprocess.PIPE
)
```

**Risk**: The `file_path` comes from user-controlled upload, but is passed to `ffprobe`. While `subprocess_exec` avoids shell injection, malicious filenames could potentially cause issues.

**Recommendation**: Validate that `file_path` exists and is within the expected upload directory. Consider using absolute paths with canonicalization.

---

### Recommendations

#### Priority 1: Critical (Before Any Production Use)

1. **Implement Real JWT Authentication**
   - Replace mock `get_current_user()` with proper JWT validation
   - Integrate with an identity provider or implement local user management
   - Ensure all protected endpoints use the authentication dependency

2. **Generate Per-Installation Encryption Salt**
   - Create random salt during installation
   - Store in secure location (e.g., separate file, environment variable)
   - Update `CredentialEncryption` to require salt parameter

3. **Require Master Key in Production**
   - Add environment check similar to JWT_SECRET_KEY validation
   - Fail startup if PENF_MASTER_KEY uses default in production

#### Priority 2: High (Before Multi-User/Multi-Tenant Deployment)

4. **Implement Tenant Access Control**
   - Create user-tenant membership model
   - Validate tenant access in `switch_to_tenant()`
   - Add audit logging for tenant switches

5. **Secure NOTIFY Channel Names**
   - Add validation regex: `^[a-z_][a-z0-9_]*$`
   - Reject any channel name not matching pattern

6. **Add Authentication to Upload Endpoints**
   - Require authentication for all file upload operations
   - Implement rate limiting per authenticated user

#### Priority 3: Medium (General Hardening)

7. **Standardize Privacy Controls Across Codepaths**
   - Ensure `app/search/` and `penf_lib/search/` use identical privacy filtering
   - Consider consolidating to single implementation (per Pass 1 recommendation)

8. **Sanitize Error Responses in Production**
   - Remove exception type names from error responses
   - Implement correlation IDs for client-side error reporting

9. **Add File Path Canonicalization**
   - Validate upload file paths are within expected directories
   - Use `Path.resolve()` to prevent directory traversal

---

## Data Flow Analysis

### External Input -> Storage Flow

```
User Request
    |
    v
[FastAPI Routes] ---- Mock Auth (VULNERABILITY)
    |
    v
[Pydantic Validation] ---- Input sanitization (OK)
    |
    v
[SQLAlchemy ORM] ---- Parameterized queries (OK)
    |
    v
[PostgreSQL + RLS] ---- Tenant isolation (NEEDS ACCESS CONTROL)
```

### Gmail OAuth Flow

```
User
    |
    v
[OAuth2 Flow] ---- Google authentication (OK)
    |
    v
[Token Receipt]
    |
    v
[Fernet Encryption] ---- Static salt (VULNERABILITY)
    |
    v
[PostgreSQL Storage] ---- Encrypted at rest (OK)
```

### Search Query Flow

```
User Query
    |
    v
[Query Parser] ---- Temporal extraction (OK)
    |
    v
[SQLAlchemy ORM] ---- Parameterized FTS/vector search (OK)
    |
    v
[Privacy Filter] ---- Role-based filtering (OK)
    |
    v
[Result Sanitization] ---- PII removal (OK)
    |
    v
[Audit Logging] ---- Access recorded (OK)
```

---

## Alignment with Constitutional Principles

| Principle | Security Implementation | Assessment |
|-----------|------------------------|------------|
| Local-First Processing | Credentials encrypted locally, no cloud key management | **Aligned** |
| User Control Over Data | Privacy levels, access controls, audit logging | **Partially Aligned** - needs tenant access control |
| Source Truth Preservation | Soft delete, immutable content model | **Aligned** |
| Human Agency Enhancement | Manual override capability in review workflows | **Aligned** |

---

## Summary Matrix

| Category | Status | Notes |
|----------|--------|-------|
| Authentication | **Incomplete** | Mock implementation in place |
| Authorization | **Partial** | Privacy filtering good, tenant access missing |
| Input Validation | **Good** | Pydantic + whitelist validation |
| SQL Injection | **Low Risk** | One channel interpolation issue |
| Credential Storage | **Good** | Encryption present, needs salt improvement |
| Audit Logging | **Good** | Search access auditing implemented |
| Error Handling | **Adequate** | Some information leakage risk |
| File Upload | **Partial** | Validation good, missing auth |

---

## Next Steps

1. Complete authentication implementation before production deployment
2. Address static salt and default key vulnerabilities
3. Implement tenant access control
4. Add authentication to upload endpoints
5. Conduct penetration testing after fixes applied
