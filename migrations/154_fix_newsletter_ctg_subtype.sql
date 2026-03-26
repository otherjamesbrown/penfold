-- Fix newsletter_ctg classification rule: CTG Post-Its is an internal corporate
-- newsletter and should be classified as NEWSLETTER_INTERNAL, not NEWSLETTER.
UPDATE classification_rules
SET content_subtype = 'NEWSLETTER_INTERNAL'
WHERE name = 'newsletter_ctg'
  AND content_subtype = 'NEWSLETTER';
