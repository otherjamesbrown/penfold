-- Seed classification rules from hardcoded source_system.go

-- Use a DO block to get the first tenant
DO $$
DECLARE
  t_id UUID;
BEGIN
  SELECT id INTO t_id FROM tenants LIMIT 1;
  IF t_id IS NULL THEN
    RAISE EXCEPTION 'No tenant found for seeding classification rules';
  END IF;

  -- Skip seeding if rules already exist (idempotent)
  IF EXISTS (SELECT 1 FROM classification_rules WHERE tenant_id = t_id LIMIT 1) THEN
    RETURN;
  END IF;

  -- Priority 10: JIRA
  INSERT INTO classification_rules (tenant_id, name, priority, content_type_scope, content_type, content_subtype, notification_source)
  VALUES (t_id, 'jira', 10, 'EMAIL', 'EMAIL', 'NOTIFICATION', 'jira');

  INSERT INTO classification_match_conditions (rule_id, field, match_type, value, case_sensitive)
  VALUES
    (currval('classification_rules_id_seq'), 'from_address', 'contains', 'jira', false),
    (currval('classification_rules_id_seq'), 'subject', 'prefix', '[TRACK-JIRA]', false),
    (currval('classification_rules_id_seq'), 'message_id', 'contains', '@Atlassian.JIRA', false);

  -- Priority 20: AHA
  INSERT INTO classification_rules (tenant_id, name, priority, content_type_scope, content_type, content_subtype, notification_source)
  VALUES (t_id, 'aha', 20, 'EMAIL', 'EMAIL', 'NOTIFICATION', 'aha');

  INSERT INTO classification_match_conditions (rule_id, field, match_type, value, case_sensitive)
  VALUES
    (currval('classification_rules_id_seq'), 'from_address', 'glob', '*@*.mailer.aha.io', false),
    (currval('classification_rules_id_seq'), 'subject', 'prefix', '[AHA]', false);

  -- Priority 30: Google Docs
  INSERT INTO classification_rules (tenant_id, name, priority, content_type_scope, content_type, content_subtype, notification_source)
  VALUES (t_id, 'google_docs', 30, 'EMAIL', 'EMAIL', 'NOTIFICATION', 'google_docs');

  INSERT INTO classification_match_conditions (rule_id, field, match_type, value, case_sensitive)
  VALUES
    (currval('classification_rules_id_seq'), 'from_address', 'glob', '*-noreply@docs.google.com', false),
    (currval('classification_rules_id_seq'), 'from_address', 'exact', 'drive-shares-dm-noreply@google.com', false);

  -- Priority 40: Webex
  INSERT INTO classification_rules (tenant_id, name, priority, content_type_scope, content_type, content_subtype, notification_source)
  VALUES (t_id, 'webex', 40, 'EMAIL', 'EMAIL', 'NOTIFICATION', 'webex');

  INSERT INTO classification_match_conditions (rule_id, field, match_type, value, case_sensitive)
  VALUES
    (currval('classification_rules_id_seq'), 'from_address', 'exact', 'messenger@webex.com', false);

  -- Priority 50: Smartsheet
  INSERT INTO classification_rules (tenant_id, name, priority, content_type_scope, content_type, content_subtype, notification_source)
  VALUES (t_id, 'smartsheet', 50, 'EMAIL', 'EMAIL', 'NOTIFICATION', 'smartsheet');

  INSERT INTO classification_match_conditions (rule_id, field, match_type, value, case_sensitive)
  VALUES
    (currval('classification_rules_id_seq'), 'from_address', 'glob', '*@*.smartsheet.com', false);

  -- Priority 60: Auto-reply
  INSERT INTO classification_rules (tenant_id, name, priority, content_type_scope, content_type, content_subtype)
  VALUES (t_id, 'auto_reply', 60, 'EMAIL', 'EMAIL', 'AUTO_REPLY');

  INSERT INTO classification_match_conditions (rule_id, field, match_type, value, case_sensitive)
  VALUES
    (currval('classification_rules_id_seq'), 'subject', 'prefix', 'Automatic reply:', false);

  -- Priority 70: Calendar cancellation
  INSERT INTO classification_rules (tenant_id, name, priority, content_type_scope, content_type, content_subtype)
  VALUES (t_id, 'calendar_cancel', 70, NULL, 'CALENDAR', 'CANCELLATION');

  INSERT INTO classification_match_conditions (rule_id, field, match_type, value, case_sensitive)
  VALUES
    (currval('classification_rules_id_seq'), 'subject', 'prefix', 'Canceled:', false),
    (currval('classification_rules_id_seq'), 'subject', 'prefix', 'Cancelled:', false);

  -- Priority 80: Calendar invite
  INSERT INTO classification_rules (tenant_id, name, priority, content_type_scope, content_type, content_subtype)
  VALUES (t_id, 'calendar_invite', 80, NULL, 'CALENDAR', 'INVITE');

  INSERT INTO classification_match_conditions (rule_id, field, match_type, value, case_sensitive)
  VALUES
    (currval('classification_rules_id_seq'), 'header:Content-Type', 'contains', 'text/calendar', false);
END $$;

-- Rollback (manual):
-- DELETE FROM classification_match_conditions WHERE rule_id IN (SELECT id FROM classification_rules);
-- DELETE FROM classification_rules;
