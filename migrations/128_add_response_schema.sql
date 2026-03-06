-- Add response_schema column for structured output enforcement
ALTER TABLE prompt_templates ADD COLUMN IF NOT EXISTS response_schema jsonb;

-- Seed triage schema (active version)
UPDATE prompt_templates SET response_schema = '{
  "type": "object",
  "properties": {
    "category": {"type": "string", "enum": ["PROJECT_UPDATE","CUSTOMER","RISK_ISSUE","ACTION_REQUEST","DECISION","INTERNAL_COMMS","PERSONAL","FYI"]},
    "importance": {"type": "string", "enum": ["HIGH","MEDIUM","LOW"]},
    "reason": {"type": "string"},
    "content_contribution": {"type": "string", "enum": ["HIGH","MEDIUM","LOW","NONE"]},
    "contribution_reason": {"type": "string"}
  },
  "required": ["category","importance","reason","content_contribution","contribution_reason"]
}'::jsonb
WHERE stage = 'triage' AND is_active = true;

-- Seed extract_ner schema (active version)
UPDATE prompt_templates SET response_schema = '{
  "type": "object",
  "properties": {
    "people": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "name": {"type": "string"},
          "role": {"type": "string"}
        },
        "required": ["name","role"]
      }
    },
    "dates": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "date": {"type": "string"},
          "context": {"type": "string"}
        },
        "required": ["date","context"]
      }
    },
    "projects": {"type": "array", "items": {"type": "string"}},
    "organisations": {"type": "array", "items": {"type": "string"}}
  },
  "required": ["people","dates","projects","organisations"]
}'::jsonb
WHERE stage = 'extract_ner' AND is_active = true;

-- Seed extract_semantic schema (active version)
UPDATE prompt_templates SET response_schema = '{
  "type": "object",
  "properties": {
    "action_items": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "assignee": {"type": "string"},
          "action": {"type": "string"},
          "due": {"type": "string"}
        },
        "required": ["assignee","action","due"]
      }
    },
    "decisions": {"type": "array", "items": {"type": "string"}},
    "risks": {"type": "array", "items": {"type": "string"}}
  },
  "required": ["action_items","decisions","risks"]
}'::jsonb
WHERE stage = 'extract_semantic' AND is_active = true;

-- Seed extract_assertions schema (active version)
UPDATE prompt_templates SET response_schema = '{
  "type": "object",
  "properties": {
    "assertions": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "type": {"type": "string"},
          "subject": {"type": "string"},
          "predicate": {"type": "string"},
          "object": {"type": "string"},
          "confidence": {"type": "number"},
          "source_text": {"type": "string"}
        },
        "required": ["type","subject","predicate","object","confidence","source_text"]
      }
    }
  },
  "required": ["assertions"]
}'::jsonb
WHERE stage = 'extract_assertions' AND is_active = true;

-- Seed classification schema (active version)
UPDATE prompt_templates SET response_schema = '{
  "type": "object",
  "properties": {
    "classifications": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "label": {"type": "string"},
          "confidence": {"type": "number"},
          "explanation": {"type": "string"}
        },
        "required": ["label","confidence","explanation"]
      }
    }
  },
  "required": ["classifications"]
}'::jsonb
WHERE stage = 'classification' AND is_active = true;
