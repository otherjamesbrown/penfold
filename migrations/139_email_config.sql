-- Migration 139: Email delivery configuration
-- NOTE: Credential values are placeholders. Real values must be inserted
-- via post-deploy step using secrets from ~/github/otherjamesbrown/secrets/gmail-credentials.json
BEGIN;

INSERT INTO pipeline_operational_config (tenant_id, key, value, description) VALUES
  ('c3170310-78bd-409c-b186-126f40bfa6ad', 'email.sender_address', 'ha27ox@gmail.com', 'Gmail sender address for digest delivery'),
  ('c3170310-78bd-409c-b186-126f40bfa6ad', 'email.oauth_client_id', '<REPLACE_FROM_SECRETS>', 'Gmail OAuth client ID'),
  ('c3170310-78bd-409c-b186-126f40bfa6ad', 'email.oauth_client_secret', '<REPLACE_FROM_SECRETS>', 'Gmail OAuth client secret'),
  ('c3170310-78bd-409c-b186-126f40bfa6ad', 'email.oauth_refresh_token', '<REPLACE_FROM_SECRETS>', 'Gmail OAuth refresh token'),
  ('c3170310-78bd-409c-b186-126f40bfa6ad', 'email.outbound_whitelist', '["james@brown.chat"]', 'JSON array of allowed email recipients')
ON CONFLICT (tenant_id, key) DO UPDATE SET value = EXCLUDED.value;

COMMIT;
