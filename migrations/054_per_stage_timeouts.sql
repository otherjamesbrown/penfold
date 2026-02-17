-- Per-stage timeout configuration keys.
-- Allows each pipeline stage to have independent timeout settings
-- instead of sharing category-based presets (embedding, llm, etc.).
--
-- Default values mirror the current category assignments:
--   triage, extract_entities, extract_assertions, embedding → embedding preset (120s/30s)
--   deep_analyze → llm preset (600s/300s)

INSERT INTO pipeline_config (key, value, value_type, description, min_value, max_value, default_value)
VALUES
  ('timeout.stage.triage.start_to_close', '120s', 'duration', 'Triage stage StartToClose timeout', '10s', '600s', '120s'),
  ('timeout.stage.triage.heartbeat', '30s', 'duration', 'Triage stage heartbeat timeout', '5s', '120s', '30s'),
  ('timeout.stage.extract_entities.start_to_close', '120s', 'duration', 'Extract entities StartToClose timeout', '10s', '600s', '120s'),
  ('timeout.stage.extract_entities.heartbeat', '30s', 'duration', 'Extract entities heartbeat timeout', '5s', '120s', '30s'),
  ('timeout.stage.extract_assertions.start_to_close', '120s', 'duration', 'Extract assertions StartToClose timeout', '10s', '600s', '120s'),
  ('timeout.stage.extract_assertions.heartbeat', '30s', 'duration', 'Extract assertions heartbeat timeout', '5s', '120s', '30s'),
  ('timeout.stage.deep_analyze.start_to_close', '600s', 'duration', 'Deep analyze StartToClose timeout', '60s', '1800s', '600s'),
  ('timeout.stage.deep_analyze.heartbeat', '300s', 'duration', 'Deep analyze heartbeat timeout', '30s', '900s', '300s'),
  ('timeout.stage.embedding.start_to_close', '120s', 'duration', 'Embedding StartToClose timeout', '10s', '600s', '120s'),
  ('timeout.stage.embedding.heartbeat', '30s', 'duration', 'Embedding heartbeat timeout', '5s', '120s', '30s')
ON CONFLICT (key) DO NOTHING;
