# Task 14: Documentation

**Status**: pending | **Phase**: 5 - Testing & Docs

## Objective

Document mTLS setup and troubleshooting.

## Output

- `docs/infrastructure/mtls-setup.md` (new)
- Update `README.md` with TLS section
- Update `docs/infrastructure/` deployment docs

## Documentation: mtls-setup.md

```markdown
# mTLS Authentication Setup

Penfold uses mutual TLS (mTLS) to authenticate CLI clients. Each client
must present a valid certificate signed by the penfold CA.

## Quick Start

### 1. Get certificates from an admin

You'll need three files:
- `ca.crt` - Certificate Authority (verifies the gateway)
- `client.crt` - Your client certificate (your identity)
- `client.key` - Your private key (keep secret!)

### 2. Install certificates

\`\`\`bash
penf cert init --from /path/to/your/certs
\`\`\`

Or manually:
\`\`\`bash
mkdir -p ~/.config/penf/certs
cp ca.crt client.crt client.key ~/.config/penf/certs/
chmod 600 ~/.config/penf/certs/client.key
\`\`\`

### 3. Verify setup

\`\`\`bash
penf cert verify
\`\`\`

## For Administrators

### Generate the CA (one-time)

\`\`\`bash
./scripts/certs/create-ca.sh ~/secrets/penfold-ca
\`\`\`

Keep `ca.key` secret! If compromised, regenerate everything.

### Generate client certificates

\`\`\`bash
./scripts/certs/create-client-cert.sh <client-name> ~/secrets/penfold-ca /tmp/new-client
\`\`\`

Securely transfer the three files to the client machine.

### Gateway configuration

\`\`\`bash
export GATEWAY_TLS_ENABLED=true
export GATEWAY_TLS_CERT=/etc/penfold/certs/server.crt
export GATEWAY_TLS_KEY=/etc/penfold/certs/server.key
export GATEWAY_TLS_CA=/etc/penfold/certs/ca.crt
export GATEWAY_TLS_CLIENT_AUTH=require
\`\`\`

## Troubleshooting

### "certificate signed by unknown authority"

Your client cert wasn't signed by the CA the gateway trusts.
- Check you have the correct `ca.crt`
- Verify with: `penf cert show`

### "connection refused"

Gateway might not be running or wrong address.
- Check: `penf config show` for server address
- Test without TLS: `penf health --insecure`

### "certificate has expired"

Your client certificate has expired.
- Check expiry: `penf cert show`
- Request new certificate from admin

### "remote error: tls: bad certificate"

Gateway rejected your certificate.
- Certificate may be revoked
- Certificate may be for different client
- Check: `penf cert verify -v`
```

## Acceptance Criteria

- [ ] mtls-setup.md covers all scenarios
- [ ] README.md mentions TLS requirement
- [ ] Troubleshooting for common errors
- [ ] Admin guide for cert generation
- [ ] CLI commands documented

## Notes

- Keep it concise, link to details
- Focus on "how do I fix this" for troubleshooting
- Include copy-pastable commands
