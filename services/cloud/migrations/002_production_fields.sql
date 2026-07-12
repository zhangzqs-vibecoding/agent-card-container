BEGIN;

ALTER TABLE generation_events
  ADD COLUMN error_code text;

ALTER TABLE card_versions
  ADD COLUMN title text NOT NULL DEFAULT '',
  ADD COLUMN description text NOT NULL DEFAULT '';

COMMIT;
