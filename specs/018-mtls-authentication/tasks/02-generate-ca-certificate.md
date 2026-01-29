# Task 02: Generate CA Certificate

**Status**: pending | **Blocked by**: 01 | **Blocks**: 03

## Objective

Run the CA tooling script to create the actual penfold CA certificate.

## Output

CA files stored securely:
- Primary: `~/github/otherjamesbrown/secrets/penfold-ca/ca.crt`
- Primary: `~/github/otherjamesbrown/secrets/penfold-ca/ca.key`

## Steps

1. Create secure directory for CA
```bash
mkdir -p ~/github/otherjamesbrown/secrets/penfold-ca
chmod 700 ~/github/otherjamesbrown/secrets/penfold-ca
```

2. Run CA generation script
```bash
cd ~/github/otherjamesbrown/penfold
./scripts/certs/create-ca.sh ~/github/otherjamesbrown/secrets/penfold-ca
```

3. Verify certificate
```bash
openssl x509 -in ~/github/otherjamesbrown/secrets/penfold-ca/ca.crt -text -noout
```

4. Copy ca.crt to gateway and CLI locations
```bash
# Gateway (for deployment)
cp ~/github/otherjamesbrown/secrets/penfold-ca/ca.crt \
   ~/github/otherjamesbrown/penfold/deploy/certs/ca.crt

# Local CLI
mkdir -p ~/.config/penf/certs
cp ~/github/otherjamesbrown/secrets/penfold-ca/ca.crt \
   ~/.config/penf/certs/ca.crt
```

## Acceptance Criteria

- [ ] CA certificate exists and is valid
- [ ] CA key is secured (600 permissions, secrets repo)
- [ ] ca.crt distributed to deploy/certs/ for gateway use
- [ ] ca.crt copied to local ~/.config/penf/certs/

## Notes

- Never commit ca.key to any repo
- ca.crt is public, safe to distribute
- Backup ca.key securely (if lost, must regenerate everything)
