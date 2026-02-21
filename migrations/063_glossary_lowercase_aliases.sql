-- Normalize existing glossary aliases to lowercase for case-insensitive JSONB containment queries.
-- The application layer now lowercases aliases on Create/Update, but existing rows need fixing.
-- Only target rows where aliases is a non-empty JSON array (skip NULL, json null, and empty arrays).
UPDATE glossary SET aliases = (
    SELECT jsonb_agg(lower(elem))
    FROM jsonb_array_elements_text(aliases) AS elem
) WHERE jsonb_typeof(aliases) = 'array' AND aliases != '[]'::jsonb;
