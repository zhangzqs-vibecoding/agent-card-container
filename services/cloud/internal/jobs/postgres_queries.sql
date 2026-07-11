-- claimJob atomically leases one available job. It must run in a transaction.
WITH candidate AS (
  SELECT id
  FROM generation_jobs
  WHERE
    status = 'queued'
    OR (status = 'running' AND lease_until <= $1)
  ORDER BY created_at, id
  FOR UPDATE SKIP LOCKED
  LIMIT 1
)
UPDATE generation_jobs AS job
SET
  status = CASE
    WHEN job.attempts >= job.max_attempts THEN 'failed'
    ELSE 'running'
  END,
  attempts = CASE
    WHEN job.attempts >= job.max_attempts THEN job.attempts
    ELSE job.attempts + 1
  END,
  lease_owner = CASE
    WHEN job.attempts >= job.max_attempts THEN NULL
    ELSE $2
  END,
  lease_until = CASE
    WHEN job.attempts >= job.max_attempts THEN NULL
    ELSE $3
  END,
  updated_at = $1
FROM candidate
WHERE job.id = candidate.id
RETURNING job.*;
