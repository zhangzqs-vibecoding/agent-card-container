BEGIN;

CREATE TABLE generation_sessions (
  id text PRIMARY KEY,
  user_id text NOT NULL,
  prompt text NOT NULL,
  target text NOT NULL,
  locale text NOT NULL,
  status text NOT NULL,
  summary_json jsonb NOT NULL,
  version_id text,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);

CREATE TABLE generation_messages (
  session_id text NOT NULL REFERENCES generation_sessions(id) ON DELETE CASCADE,
  sequence bigint NOT NULL,
  role text NOT NULL,
  content text NOT NULL,
  created_at timestamptz NOT NULL,
  PRIMARY KEY (session_id, sequence)
);

CREATE TABLE generation_events (
  session_id text NOT NULL REFERENCES generation_sessions(id) ON DELETE CASCADE,
  event_id bigint NOT NULL,
  type text NOT NULL,
  stage text NOT NULL,
  message text NOT NULL,
  progress double precision NOT NULL,
  version_id text,
  created_at timestamptz NOT NULL,
  PRIMARY KEY (session_id, event_id)
);

CREATE TABLE generation_jobs (
  id text PRIMARY KEY,
  session_id text NOT NULL UNIQUE REFERENCES generation_sessions(id) ON DELETE CASCADE,
  status text NOT NULL,
  attempts integer NOT NULL DEFAULT 0,
  max_attempts integer NOT NULL DEFAULT 3,
  lease_owner text,
  lease_until timestamptz,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);

CREATE INDEX generation_jobs_claim_idx
  ON generation_jobs (status, lease_until, created_at);

CREATE TABLE card_versions (
  version_id text PRIMARY KEY,
  card_id text NOT NULL,
  user_id text NOT NULL,
  runtime text NOT NULL,
  display_version text NOT NULL,
  artifact_object_key text NOT NULL,
  artifact_sha256 text NOT NULL,
  key_id text NOT NULL,
  preview_json jsonb NOT NULL,
  created_at timestamptz NOT NULL,
  UNIQUE (user_id, card_id, display_version)
);

COMMIT;
