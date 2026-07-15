BEGIN;

ALTER TABLE generation_sessions
  ADD COLUMN base_card_id text,
  ADD COLUMN base_version_id text;

ALTER TABLE generation_sessions
  ADD CONSTRAINT generation_sessions_base_version_pair_chk
  CHECK (
    (base_card_id IS NULL AND base_version_id IS NULL) OR
    (base_card_id IS NOT NULL AND base_version_id IS NOT NULL)
  );

COMMIT;
