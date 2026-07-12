BEGIN;

ALTER TABLE generation_sessions
  ADD COLUMN confirmed_requirement_json jsonb;

COMMIT;
