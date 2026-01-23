# Architecture Review: Security & Data Flow

**Review Date**: 2026-01-23
**Reviewer**: Architecture Review Agent
**Context Reference**: pass-00-context.md, pass-01-structure.md

---

## Summary

Penfold demonstrates a **security-conscious architecture** appropriate for a local-first personal information system. The codebase implements industry-standard cryptographic practices, proper authentication patterns, and careful secrets management. The local-first architecture inherently reduces attack surface by minimizing external exposure.

Key security strengths include:
- AES-256-GCM encryption for OAuth tokens at rest
- Constant-time comparisons for authentication tokens
- Parameterized SQL queries throughout
- Input validation with request size limits

The primary security concerns relate to:
- Machine-derived encryption keys with limited entropy sources
- Configurable table names creating theoretical SQL injection surface
- Missing CSRF protection on HTTP endpoints
- Lack of centralized input sanitization framework

---

## Previous Pass Reference

Pass 1 (Structure) identified the following items relevant to security:
- **Multi-tenancy**: Every domain type includes `TenantID` supporting isolation
- **Repository Pattern**: All database access goes through explicit repository interfaces
- **Configuration as Code**: Environment variables for secrets (no hardcoded credentials)
- **Service Boundary Ambiguity**: Some unclear deployment boundaries could impact security zones

This security review builds on those findings with focused analysis of authentication, authorization, data protection, and input handling.

---

## Findings

### Strengths

#### 1. Strong Token Encryption (AES-256-GCM)

The OAuth token encryption implementation in `/Users/james/github/otherjamesbrown/penfold/services/gmail/oauth/encryption.go` is exemplary:

```go
// AES-256 key size validation
if len(key) != aes256KeySize {
    return nil, fmt.Errorf("encryption key must be exactly %d bytes, got %d", aes256KeySize, len(key))
}

// Proper GCM mode with random nonce
nonce := make([]byte, e.gcm.NonceSize())
if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
    return nil, fmt.Errorf("generating nonce: %w", err)
}
ciphertext := e.gcm.Seal(nonce, nonce, plaintext, nil)
```

**Assessment**: Uses authenticated encryption (AEAD), cryptographically secure random nonces, and proper key size enforcement. This protects OAuth tokens at rest in PostgreSQL.

#### 2. Constant-Time Authentication Comparisons

API key validation uses constant-time comparison to prevent timing attacks (`/Users/james/github/otherjamesbrown/penfold/pkg/auth/apikey.go`):

```go
// Use constant-time comparison to prevent timing attacks
for storedKey, info := range v.keys {
    if subtle.ConstantTimeCompare([]byte(key), []byte(storedKey)) == 1 {
        return info, nil
    }
}
```

The push notification server also uses constant-time comparison for bearer token verification (`/Users/james/github/otherjamesbrown/penfold/services/gmail/push/server.go` line 249).

**Assessment**: Proper defense against timing side-channel attacks on authentication.

#### 3. Parameterized SQL Queries

All repository implementations use parameterized queries with positional placeholders:

```go
// Example from pkg/glossary/repository.go
err := r.db.QueryRow(ctx, `
    SELECT id, tenant_id, term, ...
    FROM glossary WHERE id = $1`, id).Scan(...)
```

**Assessment**: Eliminates SQL injection risk for user-controlled parameters. The consistent use of `pgx` with positional placeholders (`$1`, `$2`, etc.) is the correct approach.

#### 4. Request Size Limits

HTTP endpoints enforce request size limits to prevent denial-of-service:

```go
// services/gmail/push/server.go
MaxRequestSize: 1 * 1024 * 1024, // 1MB

// Applied on incoming requests
r.Body = http.MaxBytesReader(w, r.Body, s.config.MaxRequestSize)
```

**Assessment**: Protects against memory exhaustion attacks on endpoints accepting request bodies.

#### 5. Credentials File Permissions

The CLI credential store uses restrictive file permissions:

```go
// cmd/penf/credentials/credentials.go
if err := os.WriteFile(credPath, data, 0600); err != nil { ... }
```

**Assessment**: Credentials file readable only by the owner, preventing local privilege escalation via credential theft.

#### 6. JWT Signing Method Validation

JWT validation explicitly checks the signing algorithm:

```go
// pkg/auth/auth.go
if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
    return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
}
```

**Assessment**: Prevents algorithm confusion attacks where an attacker could trick the validator into using a different (weaker) algorithm.

#### 7. Sensitive Data Redaction in API Responses

Token listing redacts sensitive values:

```go
// services/gmail/oauth/storage.go - ListTokens()
token.AccessToken = "[REDACTED]"
token.RefreshToken = "[REDACTED]"
```

**Assessment**: Prevents accidental exposure of credentials through admin endpoints.

---

### Concerns

#### 1. Machine-Derived Encryption Key (Medium)

**Location**: `/Users/james/github/otherjamesbrown/penfold/cmd/penf/credentials/credentials.go`

The CLI credential encryption key is derived from machine-specific but predictable data:

```go
func deriveEncryptionKey() ([]byte, error) {
    var keyMaterial strings.Builder
    hostname, _ := os.Hostname()
    keyMaterial.WriteString(hostname)
    keyMaterial.WriteString(os.Getenv("USER"))
    keyMaterial.WriteString(runtime.GOOS)
    keyMaterial.WriteString(runtime.GOARCH)
    home, _ := os.UserHomeDir()
    keyMaterial.WriteString(home)

    hash := sha256.Sum256([]byte(keyMaterial.String()))
    return hash[:], nil
}
```

**Risk**: An attacker with knowledge of the machine characteristics can reconstruct the encryption key. All inputs (hostname, username, OS, architecture, home directory) are easily discoverable.

**Severity**: Medium - Requires local access, but undermines the encryption protection.

**Recommendation**:
- Use macOS Keychain or system keyring for key storage
- Alternatively, add a user-specific secret (prompted on first use) to the key derivation
- Consider using PBKDF2 or Argon2 for key derivation instead of a single SHA-256 hash

#### 2. Configurable Table Names in SQL (Low)

**Location**: `/Users/james/github/otherjamesbrown/penfold/services/gmail/oauth/storage.go`, `/Users/james/github/otherjamesbrown/penfold/services/gmail/sync/state.go`, `/Users/james/github/otherjamesbrown/penfold/services/gmail/push/storage.go`

Table names are configurable and interpolated into SQL queries:

```go
tableName := cfg.TableName
if tableName == "" {
    tableName = "oauth_tokens"
}

query := fmt.Sprintf(`DELETE FROM %s WHERE tenant_id = $1`, s.tableName)
```

**Risk**: If `TableName` is derived from user input (it currently is not), this could enable SQL injection. The current implementation only accepts configuration values, so the risk is theoretical.

**Severity**: Low - Configuration-only, but the pattern is brittle.

**Recommendation**:
- Add table name validation (alphanumeric and underscore only)
- Consider using a whitelist of allowed table names
- Document that table names must not come from user input

#### 3. Missing CSRF Protection (Medium)

**Location**: HTTP handlers in `/Users/james/github/otherjamesbrown/penfold/services/gmail/push/server.go`, `/Users/james/github/otherjamesbrown/penfold/services/gateway/health/aggregator.go`

HTTP endpoints do not implement CSRF tokens for state-changing operations.

**Risk**: If the gateway exposes HTTP endpoints with cookie-based authentication, attackers could perform cross-site request forgery.

**Severity**: Medium - The system currently uses bearer tokens (not cookies), reducing immediate risk. However, if HTTP-based authentication is added, CSRF would be exploitable.

**Recommendation**:
- Add CSRF protection middleware for any endpoints that accept browser-based requests
- Ensure SameSite cookie attributes are set if cookies are used
- Consider requiring API key or JWT for all state-changing operations

#### 4. Absence of Input Sanitization Framework (Low)

**Location**: Various handlers and repositories

While SQL injection is prevented via parameterized queries, there is no centralized framework for input validation and sanitization.

**Risk**: Inconsistent validation across endpoints could allow malformed data into the system. While not immediately exploitable, it increases the likelihood of logic errors or data corruption.

**Severity**: Low - Defense in depth concern rather than active vulnerability.

**Recommendation**:
- Implement validation helpers for common input types (UUIDs, emails, dates)
- Consider using a validation library (e.g., `go-playground/validator`)
- Add input length limits to string fields at the repository layer

#### 5. Command Execution in Feedback (Low)

**Location**: `/Users/james/github/otherjamesbrown/penfold/cmd/penf/cmd/feedback.go`

The feedback command executes `gh` CLI with user-provided arguments:

```go
args := []string{
    "issue", "create",
    "--repo", FeedbackGitHubRepo,
    "--title", title,
    "--body", body,
}
cmd := exec.Command("gh", args...)
```

**Risk**: User-provided title and body are passed to `gh` CLI. While `exec.Command` uses argument arrays (not shell expansion), the `gh` CLI itself could interpret special characters.

**Severity**: Low - The `exec.Command` function properly separates arguments, preventing shell injection. The risk is limited to how `gh` interprets its arguments.

**Recommendation**:
- Add length limits to title and body
- Consider sanitizing or escaping special characters
- Document the trust boundary (user-provided input flows to GitHub API)

#### 6. No Rate Limiting on Push Endpoint (Medium)

**Location**: `/Users/james/github/otherjamesbrown/penfold/services/gmail/push/server.go`

The push notification endpoint has authentication and request size limits but no rate limiting.

**Risk**: An attacker with valid credentials could flood the endpoint, causing resource exhaustion.

**Severity**: Medium - Requires valid auth token, but could cause denial of service.

**Recommendation**:
- Add per-source IP rate limiting
- Consider implementing backpressure or circuit breaker patterns
- Add metrics for request rates to detect abuse

#### 7. Database Password in Connection String (Low)

**Location**: `/Users/james/github/otherjamesbrown/penfold/pkg/config/config.go`

Database passwords are included in connection strings:

```go
return fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s sslmode=disable",
    d.Host, d.Port, d.Name, d.User, d.Password)
```

**Risk**: Connection strings may appear in logs, error messages, or stack traces, potentially exposing the password.

**Severity**: Low - This is standard practice for PostgreSQL, but care must be taken.

**Recommendation**:
- Ensure connection strings are never logged
- Consider using `.pgpass` file or environment variable authentication
- Add log sanitization middleware that redacts passwords

---

### Recommendations

#### High Priority

1. **Improve Credential Key Derivation**: Replace the machine-characteristic-based key derivation with OS keychain integration (macOS Keychain, Linux Secret Service) or require a user-provided passphrase component.

2. **Add CSRF Protection**: Implement CSRF middleware for any HTTP endpoints that could be accessed from browsers. Even if currently API-only, this prevents future vulnerabilities.

3. **Add Push Endpoint Rate Limiting**: Implement rate limiting on the Gmail push notification endpoint to prevent abuse by compromised credentials.

#### Medium Priority

4. **Validate Configurable Table Names**: Add validation to ensure table names contain only safe characters (alphanumeric and underscore).

5. **Create Input Validation Helpers**: Build a validation library for common input types used across the codebase to ensure consistent validation.

6. **Add Connection String Sanitization**: Ensure database connection strings are never written to logs by adding log filtering or using alternative authentication methods.

#### Low Priority

7. **Audit Logging Framework**: Implement security-focused audit logging for authentication events, authorization failures, and sensitive data access.

8. **Add Security Headers**: Ensure all HTTP responses include security headers (X-Content-Type-Options, X-Frame-Options, etc.) even for API endpoints.

9. **Document Security Trust Boundaries**: Create a security architecture document showing where user input enters the system and how it flows through to storage and output.

---

## Data Flow Analysis

### External Input Entry Points

| Entry Point | Input Type | Validation | Storage Destination |
|-------------|-----------|------------|---------------------|
| Gmail Push Webhook | JSON notification | Signature + structure validation | Triggers sync workflow |
| gRPC Gateway | Protocol buffers | Proto schema validation | Various repositories |
| CLI Commands | User arguments | Cobra flag validation | Config files, API calls |
| OAuth Callback | Authorization code | State parameter validation | Encrypted token storage |
| Health Endpoints | None (read-only) | N/A | None |

### Sensitive Data Handling

| Data Type | At Rest | In Transit | Access Control |
|-----------|---------|------------|----------------|
| OAuth Tokens | AES-256-GCM encrypted | TLS (to Google) | Per-tenant isolation |
| CLI Credentials | AES-GCM (machine key) | Local only | File permissions (0600) |
| Database Password | Environment variable | PostgreSQL protocol | Server-side only |
| JWT Tokens | Not persisted | gRPC metadata | HMAC signed |
| Email Content | PostgreSQL (immutable) | Local network | Tenant ID filtering |

### Privacy Alignment

The security architecture aligns well with the constitutional principle of "Local-First, Cloud-Strategic":
- Tokens encrypted at rest in local PostgreSQL
- No credentials stored in cloud services
- All processing occurs on local infrastructure
- Cloud contact only for Gmail API sync

---

## Conclusion

Penfold demonstrates solid security fundamentals for a personal information system. The use of AES-256-GCM for token encryption, constant-time authentication comparisons, and parameterized SQL queries shows security awareness in the development process.

The primary areas for improvement relate to defense-in-depth measures: strengthening the CLI credential encryption key derivation, adding CSRF protection for future-proofing, and implementing rate limiting on exposed endpoints.

Given the local-first architecture and single-user deployment model, the current security posture is appropriate. The recommendations focus on strengthening existing controls rather than addressing critical vulnerabilities.

**Overall Security Assessment**: Good - appropriate for local-first personal system with some improvements recommended for defense in depth.
