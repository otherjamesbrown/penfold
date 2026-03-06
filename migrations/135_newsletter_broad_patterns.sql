-- +goose Up
DO $$
DECLARE
  t_id UUID;
  t_rec RECORD;
BEGIN
  FOR t_rec IN SELECT id FROM tenants LOOP
    t_id := t_rec.id;

    -- Priority 85: Newsletter broad patterns
    -- Catch newsletters by common sender naming conventions
    IF NOT EXISTS (SELECT 1 FROM classification_rules WHERE tenant_id = t_id AND name = 'newsletter_comms_pattern') THEN
      INSERT INTO classification_rules (tenant_id, name, priority, content_type_scope, content_type, content_subtype)
      VALUES (t_id, 'newsletter_comms_pattern', 85, 'EMAIL', 'EMAIL', 'NEWSLETTER');
      INSERT INTO classification_match_conditions (rule_id, field, match_type, value, case_sensitive)
      VALUES (currval('classification_rules_id_seq'), 'from_address', 'glob', '*comms@*', false);
    END IF;

    IF NOT EXISTS (SELECT 1 FROM classification_rules WHERE tenant_id = t_id AND name = 'newsletter_wave_pattern') THEN
      INSERT INTO classification_rules (tenant_id, name, priority, content_type_scope, content_type, content_subtype)
      VALUES (t_id, 'newsletter_wave_pattern', 85, 'EMAIL', 'EMAIL', 'NEWSLETTER');
      INSERT INTO classification_match_conditions (rule_id, field, match_type, value, case_sensitive)
      VALUES (currval('classification_rules_id_seq'), 'from_address', 'glob', '*wave@*', false);
    END IF;

    IF NOT EXISTS (SELECT 1 FROM classification_rules WHERE tenant_id = t_id AND name = 'newsletter_addr_pattern') THEN
      INSERT INTO classification_rules (tenant_id, name, priority, content_type_scope, content_type, content_subtype)
      VALUES (t_id, 'newsletter_addr_pattern', 85, 'EMAIL', 'EMAIL', 'NEWSLETTER');
      INSERT INTO classification_match_conditions (rule_id, field, match_type, value, case_sensitive)
      VALUES (currval('classification_rules_id_seq'), 'from_address', 'glob', '*newsletter@*', false);
    END IF;

  END LOOP;
END $$;

-- +goose Down
DELETE FROM classification_match_conditions WHERE rule_id IN (
  SELECT id FROM classification_rules WHERE name IN (
    'newsletter_comms_pattern', 'newsletter_wave_pattern', 'newsletter_addr_pattern'
  )
);
DELETE FROM classification_rules WHERE name IN (
  'newsletter_comms_pattern', 'newsletter_wave_pattern', 'newsletter_addr_pattern'
);
