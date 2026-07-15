BEGIN;

ALTER TABLE generation_jobs
  ADD COLUMN card_id text,
  ADD COLUMN version_id text,
  ADD COLUMN available_at timestamptz;

UPDATE generation_jobs
SET available_at = created_at
WHERE available_at IS NULL;

ALTER TABLE generation_jobs
  ALTER COLUMN available_at SET NOT NULL,
  ADD CONSTRAINT generation_jobs_publication_pair CHECK (
    (card_id IS NULL AND version_id IS NULL) OR
    (card_id IS NOT NULL AND version_id IS NOT NULL)
  );

DROP INDEX generation_jobs_claim_idx;
CREATE INDEX generation_jobs_claim_idx
  ON generation_jobs (status, available_at, lease_until, created_at);

COMMIT;
