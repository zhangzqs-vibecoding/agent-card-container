BEGIN;

ALTER TABLE generation_sessions
  ADD CONSTRAINT generation_sessions_confirmed_requirement_required
  CHECK (
    status NOT IN ('queued', 'generating', 'validating', 'ready', 'failed')
    OR confirmed_requirement_json IS NOT NULL
  ) NOT VALID;

COMMIT;
