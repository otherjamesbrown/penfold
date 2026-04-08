-- Add unique constraint on (tenant_id, name) for classification_rules.
-- Enables the gateway to return AlreadyExists on duplicate rule creation.
ALTER TABLE classification_rules
    ADD CONSTRAINT uq_classification_rules_tenant_name UNIQUE (tenant_id, name);

-- Rollback: ALTER TABLE classification_rules DROP CONSTRAINT uq_classification_rules_tenant_name;
