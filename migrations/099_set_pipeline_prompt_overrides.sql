-- pf-14f931: Set prompt_override on notification and newsletter pipeline definitions.
-- Notification pipeline stages use prompt version 2 (notification-tailored).
-- Newsletter pipeline stages use prompt version 3 (newsletter-tailored).

UPDATE pipeline_definitions SET prompt_override = 2
WHERE pipeline = 'notification' AND stage IN ('triage', 'summarize', 'extract_semantic');

UPDATE pipeline_definitions SET prompt_override = 3
WHERE pipeline = 'newsletter' AND stage IN ('triage', 'extract_semantic');
