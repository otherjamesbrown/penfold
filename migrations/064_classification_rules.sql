-- +goose Up
CREATE TABLE classification_rules (
  id SERIAL PRIMARY KEY,
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  name VARCHAR(50) NOT NULL,
  priority INTEGER NOT NULL,
  content_type_scope VARCHAR(20),
  content_type VARCHAR(20) NOT NULL,
  content_subtype VARCHAR(20) NOT NULL,
  notification_source VARCHAR(50),
  active BOOLEAN DEFAULT true,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_classification_rules_tenant ON classification_rules(tenant_id);
CREATE INDEX idx_classification_rules_active ON classification_rules(tenant_id, active) WHERE active = true;

CREATE TABLE classification_match_conditions (
  id SERIAL PRIMARY KEY,
  rule_id INTEGER NOT NULL REFERENCES classification_rules(id) ON DELETE CASCADE,
  field VARCHAR(100) NOT NULL,
  match_type VARCHAR(20) NOT NULL,
  value VARCHAR(255) NOT NULL,
  case_sensitive BOOLEAN DEFAULT false
);

CREATE INDEX idx_classification_conditions_rule ON classification_match_conditions(rule_id);

-- +goose Down
DROP TABLE IF EXISTS classification_match_conditions;
DROP TABLE IF EXISTS classification_rules;
