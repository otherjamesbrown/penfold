-- Seed newsletter detection rule for the classification rule engine.
-- Priority 90: lower than notification rules (10-50), so notifications
-- with List-Unsubscribe headers are not reclassified as newsletters.
-- Part of epic pf-778226, task pf-04cabe.

DO $$
DECLARE
  t_id UUID;
BEGIN
  SELECT id INTO t_id FROM tenants LIMIT 1;
  IF t_id IS NULL THEN
    RAISE EXCEPTION 'No tenant found for seeding newsletter classification rule';
  END IF;

  -- Skip if newsletter rule already exists (idempotent)
  IF EXISTS (SELECT 1 FROM classification_rules WHERE tenant_id = t_id AND name = 'newsletter' LIMIT 1) THEN
    RETURN;
  END IF;

  -- Priority 90: Newsletter
  INSERT INTO classification_rules (tenant_id, name, priority, content_type_scope, content_type, content_subtype)
  VALUES (t_id, 'newsletter', 90, 'EMAIL', 'EMAIL', 'NEWSLETTER');

  INSERT INTO classification_match_conditions (rule_id, field, match_type, value, case_sensitive)
  VALUES
    (currval('classification_rules_id_seq'), 'header:Precedence', 'exact', 'bulk', false),
    (currval('classification_rules_id_seq'), 'header:Precedence', 'exact', 'list', false),
    (currval('classification_rules_id_seq'), 'header:List-Unsubscribe', 'exists', '', false),
    (currval('classification_rules_id_seq'), 'subject', 'contains', 'Newsletter', false),
    (currval('classification_rules_id_seq'), 'subject', 'contains', 'Weekly Digest', false);
END $$;

-- Rollback (manual):
-- DELETE FROM classification_match_conditions WHERE rule_id IN (
--   SELECT id FROM classification_rules WHERE name = 'newsletter'
-- );
-- DELETE FROM classification_rules WHERE name = 'newsletter';
