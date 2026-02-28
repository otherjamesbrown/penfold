-- Wave 3 classification rules: calendar responses, notifications, newsletters
-- Also adds support@aha.io condition to existing aha rule

-- Seed wave 3 rules for ALL tenants (not just the first one).
-- Each tenant gets its own copy of the rules, matching the per-tenant
-- isolation pattern used by the classification engine.
DO $$
DECLARE
  t_id UUID;
  r_id INTEGER;
  t_rec RECORD;
BEGIN
  FOR t_rec IN SELECT id FROM tenants LOOP
    t_id := t_rec.id;

  -- Priority 7: Calendar Response — accept
  IF NOT EXISTS (SELECT 1 FROM classification_rules WHERE tenant_id = t_id AND name = 'calendar_accept') THEN
    INSERT INTO classification_rules (tenant_id, name, priority, content_type_scope, content_type, content_subtype, notification_source)
    VALUES (t_id, 'calendar_accept', 7, 'EMAIL', 'EMAIL', 'CALENDAR_RESPONSE', NULL);

    INSERT INTO classification_match_conditions (rule_id, field, match_type, value, case_sensitive)
    VALUES
      (currval('classification_rules_id_seq'), 'subject', 'prefix', 'Accepted:', false);
  END IF;

  -- Priority 7: Calendar Response — decline
  IF NOT EXISTS (SELECT 1 FROM classification_rules WHERE tenant_id = t_id AND name = 'calendar_decline') THEN
    INSERT INTO classification_rules (tenant_id, name, priority, content_type_scope, content_type, content_subtype, notification_source)
    VALUES (t_id, 'calendar_decline', 7, 'EMAIL', 'EMAIL', 'CALENDAR_RESPONSE', NULL);

    INSERT INTO classification_match_conditions (rule_id, field, match_type, value, case_sensitive)
    VALUES
      (currval('classification_rules_id_seq'), 'subject', 'prefix', 'Declined:', false);
  END IF;

  -- Priority 7: Calendar Response — tentative
  IF NOT EXISTS (SELECT 1 FROM classification_rules WHERE tenant_id = t_id AND name = 'calendar_tentative') THEN
    INSERT INTO classification_rules (tenant_id, name, priority, content_type_scope, content_type, content_subtype, notification_source)
    VALUES (t_id, 'calendar_tentative', 7, 'EMAIL', 'EMAIL', 'CALENDAR_RESPONSE', NULL);

    INSERT INTO classification_match_conditions (rule_id, field, match_type, value, case_sensitive)
    VALUES
      (currval('classification_rules_id_seq'), 'subject', 'prefix', 'Tentative:', false);
  END IF;

  -- Priority 7: Calendar Update
  IF NOT EXISTS (SELECT 1 FROM classification_rules WHERE tenant_id = t_id AND name = 'calendar_update') THEN
    INSERT INTO classification_rules (tenant_id, name, priority, content_type_scope, content_type, content_subtype, notification_source)
    VALUES (t_id, 'calendar_update', 7, 'EMAIL', 'EMAIL', 'CALENDAR_UPDATE', NULL);

    INSERT INTO classification_match_conditions (rule_id, field, match_type, value, case_sensitive)
    VALUES
      (currval('classification_rules_id_seq'), 'subject', 'prefix', 'Updated invitation:', false);
  END IF;

  -- Priority 5: Google security notification
  IF NOT EXISTS (SELECT 1 FROM classification_rules WHERE tenant_id = t_id AND name = 'google_security') THEN
    INSERT INTO classification_rules (tenant_id, name, priority, content_type_scope, content_type, content_subtype, notification_source)
    VALUES (t_id, 'google_security', 5, 'EMAIL', 'EMAIL', 'NOTIFICATION', 'google');

    INSERT INTO classification_match_conditions (rule_id, field, match_type, value, case_sensitive)
    VALUES
      (currval('classification_rules_id_seq'), 'from_address', 'exact', 'no-reply@accounts.google.com', false);
  END IF;

  -- Priority 5: IT/Akamai notification
  IF NOT EXISTS (SELECT 1 FROM classification_rules WHERE tenant_id = t_id AND name = 'it_notification') THEN
    INSERT INTO classification_rules (tenant_id, name, priority, content_type_scope, content_type, content_subtype, notification_source)
    VALUES (t_id, 'it_notification', 5, 'EMAIL', 'EMAIL', 'NOTIFICATION', 'akamai');

    INSERT INTO classification_match_conditions (rule_id, field, match_type, value, case_sensitive)
    VALUES
      (currval('classification_rules_id_seq'), 'from_address', 'exact', 'thesolutioncenter@akamai.com', false);
  END IF;

  -- Priority 8: Newsletter — Akamai Spark
  IF NOT EXISTS (SELECT 1 FROM classification_rules WHERE tenant_id = t_id AND name = 'newsletter_akamai_spark') THEN
    INSERT INTO classification_rules (tenant_id, name, priority, content_type_scope, content_type, content_subtype, notification_source)
    VALUES (t_id, 'newsletter_akamai_spark', 8, 'EMAIL', 'EMAIL', 'NEWSLETTER', NULL);

    INSERT INTO classification_match_conditions (rule_id, field, match_type, value, case_sensitive)
    VALUES
      (currval('classification_rules_id_seq'), 'from_address', 'exact', 'AkamaiSpark@akamai.com', false);
  END IF;

  -- Priority 8: Newsletter — Akamai Wave
  IF NOT EXISTS (SELECT 1 FROM classification_rules WHERE tenant_id = t_id AND name = 'newsletter_akamai_wave') THEN
    INSERT INTO classification_rules (tenant_id, name, priority, content_type_scope, content_type, content_subtype, notification_source)
    VALUES (t_id, 'newsletter_akamai_wave', 8, 'EMAIL', 'EMAIL', 'NEWSLETTER', NULL);

    INSERT INTO classification_match_conditions (rule_id, field, match_type, value, case_sensitive)
    VALUES
      (currval('classification_rules_id_seq'), 'from_address', 'exact', 'AkamaiWave@akamai.com', false);
  END IF;

  -- Priority 8: Newsletter — EMEA
  IF NOT EXISTS (SELECT 1 FROM classification_rules WHERE tenant_id = t_id AND name = 'newsletter_emea') THEN
    INSERT INTO classification_rules (tenant_id, name, priority, content_type_scope, content_type, content_subtype, notification_source)
    VALUES (t_id, 'newsletter_emea', 8, 'EMAIL', 'EMAIL', 'NEWSLETTER', NULL);

    INSERT INTO classification_match_conditions (rule_id, field, match_type, value, case_sensitive)
    VALUES
      (currval('classification_rules_id_seq'), 'from_address', 'exact', 'EMEA_newsletter@akamai.com', false);
  END IF;

  -- Priority 8: Newsletter — Engineering Learning
  IF NOT EXISTS (SELECT 1 FROM classification_rules WHERE tenant_id = t_id AND name = 'newsletter_englearn') THEN
    INSERT INTO classification_rules (tenant_id, name, priority, content_type_scope, content_type, content_subtype, notification_source)
    VALUES (t_id, 'newsletter_englearn', 8, 'EMAIL', 'EMAIL', 'NEWSLETTER', NULL);

    INSERT INTO classification_match_conditions (rule_id, field, match_type, value, case_sensitive)
    VALUES
      (currval('classification_rules_id_seq'), 'from_address', 'exact', 'EngLearn@akamai.com', false);
  END IF;

  -- Priority 8: Newsletter — CTG Comms
  IF NOT EXISTS (SELECT 1 FROM classification_rules WHERE tenant_id = t_id AND name = 'newsletter_ctg') THEN
    INSERT INTO classification_rules (tenant_id, name, priority, content_type_scope, content_type, content_subtype, notification_source)
    VALUES (t_id, 'newsletter_ctg', 8, 'EMAIL', 'EMAIL', 'NEWSLETTER', NULL);

    INSERT INTO classification_match_conditions (rule_id, field, match_type, value, case_sensitive)
    VALUES
      (currval('classification_rules_id_seq'), 'from_address', 'exact', 'ctgcomms@akamai.com', false);
  END IF;

  -- Fix: Add support@aha.io condition to existing aha rule
  INSERT INTO classification_match_conditions (rule_id, field, match_type, value, case_sensitive)
  SELECT r.id, 'from_address', 'exact', 'support@aha.io', false
  FROM classification_rules r
  WHERE r.tenant_id = t_id
    AND r.name = 'aha'
    AND NOT EXISTS (
      SELECT 1 FROM classification_match_conditions mc
      WHERE mc.rule_id = r.id AND mc.field = 'from_address' AND mc.value = 'support@aha.io'
    );

  END LOOP; -- end tenant loop
END $$;

-- Rollback (manual):
-- DELETE FROM classification_match_conditions WHERE rule_id IN (
--   SELECT id FROM classification_rules WHERE name IN (
--     'calendar_accept', 'calendar_decline', 'calendar_tentative', 'calendar_update',
--     'google_security', 'it_notification',
--     'newsletter_akamai_spark', 'newsletter_akamai_wave', 'newsletter_emea',
--     'newsletter_englearn', 'newsletter_ctg'
--   )
-- );
-- DELETE FROM classification_rules WHERE name IN (
--   'calendar_accept', 'calendar_decline', 'calendar_tentative', 'calendar_update',
--   'google_security', 'it_notification',
--   'newsletter_akamai_spark', 'newsletter_akamai_wave', 'newsletter_emea',
--   'newsletter_englearn', 'newsletter_ctg'
-- );
-- DELETE FROM classification_match_conditions
-- WHERE rule_id = (SELECT id FROM classification_rules WHERE name = 'aha')
--   AND field = 'from_address' AND value = 'support@aha.io';
