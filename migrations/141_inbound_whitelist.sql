INSERT INTO pipeline_operational_config (tenant_id, key, value, description)
SELECT tenant_id, 'email.inbound_whitelist', '["james@brown.chat","jabrown@akamai.com"]',
       'JSON array of allowed inbound sender addresses'
FROM (SELECT DISTINCT tenant_id FROM pipeline_operational_config) t
ON CONFLICT (tenant_id, key) DO UPDATE SET value = EXCLUDED.value;
