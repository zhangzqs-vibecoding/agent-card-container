BEGIN;

ALTER TABLE generation_sessions
  VALIDATE CONSTRAINT generation_sessions_confirmed_requirement_required;

COMMIT;
