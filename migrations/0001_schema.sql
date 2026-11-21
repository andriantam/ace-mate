-- AceMate database schema. Applied once at startup.
--
-- Written to be safe to re-run (IF NOT EXISTS everywhere), so it also upgrades a database
-- created by an earlier pre-release schema. Add later changes as 0002_*.sql and so on;
-- never edit this file once it has shipped.

-- ── Identity and sessions ───────────────────────────────────────────
-- Not granted to the restricted app role: only the server's own code touches them.

CREATE TABLE IF NOT EXISTS users (
  id                    text PRIMARY KEY,
  email                 text NOT NULL DEFAULT '',
  name                  text NOT NULL DEFAULT '',
  image                 text NOT NULL DEFAULT '',
  given_name            text NOT NULL DEFAULT '',
  family_name           text NOT NULL DEFAULT '',
  locale                text NOT NULL DEFAULT '',
  email_verified        boolean NOT NULL DEFAULT false,
  -- Only filled when AUTH_GOOGLE_BIRTHDAY is on and the user grants it.
  -- 'YYYY-MM-DD', or '--MM-DD' when the provider shares no year.
  birthday              text NOT NULL DEFAULT '',
  -- The Terms and Privacy Policy version accepted. A new version asks everyone again.
  terms_version         text NOT NULL DEFAULT '',
  terms_accepted_at     timestamptz,
  -- Marketing is a separate, optional consent, never bundled with the terms.
  marketing_opt_in      boolean NOT NULL DEFAULT false,
  marketing_updated_at  timestamptz,
  created_at            timestamptz NOT NULL DEFAULT now()
);

-- One row per (provider, subject). Identity only: provider tokens are never stored.
CREATE TABLE IF NOT EXISTS accounts (
  provider    text NOT NULL,
  subject     text NOT NULL,
  user_id     text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  PRIMARY KEY (provider, subject)
);
CREATE INDEX IF NOT EXISTS accounts_user_idx ON accounts (user_id);

-- Opaque session tokens. Only the SHA-256 hash is stored.
CREATE TABLE IF NOT EXISTS sessions (
  token_hash  bytea PRIMARY KEY,
  user_id     text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at  timestamptz NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS sessions_user_idx ON sessions (user_id);

-- ── User data (row-level security) ──────────────────────────────────

-- The merged profile that journeys are built from.
CREATE TABLE IF NOT EXISTS profile (
  user_id     text PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  data        jsonb NOT NULL,
  updated_at  timestamptz NOT NULL DEFAULT now()
);

-- Documents the user added: files (PDF, Word, text), pasted text and website links.
-- Only what the AI extracted is kept here, never the original file.
CREATE TABLE IF NOT EXISTS document (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id       text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind          text NOT NULL CHECK (kind IN ('pdf', 'docx', 'text', 'link')),
  name          text NOT NULL DEFAULT '',
  url           text,
  status        text NOT NULL DEFAULT 'processing' CHECK (status IN ('processing', 'ready', 'failed')),
  error         text,
  extracted     jsonb,
  verification  jsonb,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS document_user_idx ON document (user_id, created_at);

-- A job application and its journey.
CREATE TABLE IF NOT EXISTS job (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  url         text,
  title       text NOT NULL DEFAULT 'Untitled role',
  company     text NOT NULL DEFAULT '',
  notes       text NOT NULL DEFAULT '',
  jd_text     text,
  status      text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'ready', 'failed')),
  error       text,
  journey     jsonb,
  people      jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS job_user_idx ON job (user_id, created_at DESC);

-- Ticked tasks and passed tests in a journey.
CREATE TABLE IF NOT EXISTS progress (
  job_id      uuid NOT NULL REFERENCES job(id) ON DELETE CASCADE,
  user_id     text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  item        text NOT NULL,
  done        boolean NOT NULL,
  updated_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (job_id, item)
);

-- ── Background work ─────────────────────────────────────────────────
-- A queue of work for the AI (reading a document, writing a journey). Any server instance
-- can claim a task; a claim is a lease that the worker renews while it runs, so a task
-- whose server died is picked up again once the lease runs out. Like the identity tables,
-- these are only used by the server's worker code and are not granted to the app role;
-- every read on behalf of a user filters by user_id.
CREATE TABLE IF NOT EXISTS task (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id       text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind          text NOT NULL,            -- document | journey | fill
  subject       uuid NOT NULL,            -- the document or job it works on
  status        text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'done', 'failed')),
  attempts      int  NOT NULL DEFAULT 0,
  max_attempts  int  NOT NULL DEFAULT 3,
  run_after     timestamptz NOT NULL DEFAULT now(),
  lease_until   timestamptz,
  last_error    text,
  -- The uploaded file or text, kept only until the task finishes, then cleared.
  input         bytea,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS task_ready_idx   ON task (run_after) WHERE status = 'queued';
CREATE INDEX IF NOT EXISTS task_lease_idx   ON task (lease_until) WHERE status = 'running';
CREATE INDEX IF NOT EXISTS task_subject_idx ON task (user_id, subject, created_at DESC);

-- Progress events of a task, polled by the page that started it.
CREATE TABLE IF NOT EXISTS task_event (
  id          bigserial PRIMARY KEY,
  task_id     uuid NOT NULL REFERENCES task(id) ON DELETE CASCADE,
  data        jsonb NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS task_event_task_idx ON task_event (task_id, id);

-- ── Row-level security ──────────────────────────────────────────────
-- Every user-data query runs in a transaction that does
--   SET LOCAL ROLE app_user;  SELECT set_config('app.user_id', <session user>, true);
-- The policies then limit every read and write to that user's rows, so a bug in
-- application code (a missing WHERE clause) cannot leak another user's data.
-- FORCE makes the policies apply to the table owner as well.

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app_user') THEN
    BEGIN
      CREATE ROLE app_user NOLOGIN NOSUPERUSER NOBYPASSRLS;
    EXCEPTION WHEN insufficient_privilege THEN
      RAISE NOTICE 'could not create role app_user; relying on FORCE ROW LEVEL SECURITY for the owner role';
    END;
  END IF;
END $$;

DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app_user') THEN
    GRANT app_user TO CURRENT_USER;
    GRANT USAGE ON SCHEMA public TO app_user;
    GRANT SELECT, INSERT, UPDATE, DELETE ON profile, document, job, progress TO app_user;
  END IF;
END $$;

ALTER TABLE profile  ENABLE ROW LEVEL SECURITY;
ALTER TABLE profile  FORCE  ROW LEVEL SECURITY;
ALTER TABLE document ENABLE ROW LEVEL SECURITY;
ALTER TABLE document FORCE  ROW LEVEL SECURITY;
ALTER TABLE job      ENABLE ROW LEVEL SECURITY;
ALTER TABLE job      FORCE  ROW LEVEL SECURITY;
ALTER TABLE progress ENABLE ROW LEVEL SECURITY;
ALTER TABLE progress FORCE  ROW LEVEL SECURITY;

DROP POLICY IF EXISTS profile_owner ON profile;
CREATE POLICY profile_owner ON profile
  USING (user_id = current_setting('app.user_id', true))
  WITH CHECK (user_id = current_setting('app.user_id', true));

DROP POLICY IF EXISTS document_owner ON document;
CREATE POLICY document_owner ON document
  USING (user_id = current_setting('app.user_id', true))
  WITH CHECK (user_id = current_setting('app.user_id', true));

DROP POLICY IF EXISTS job_owner ON job;
CREATE POLICY job_owner ON job
  USING (user_id = current_setting('app.user_id', true))
  WITH CHECK (user_id = current_setting('app.user_id', true));

DROP POLICY IF EXISTS progress_owner ON progress;
CREATE POLICY progress_owner ON progress
  USING (user_id = current_setting('app.user_id', true))
  WITH CHECK (
    user_id = current_setting('app.user_id', true)
    AND EXISTS (SELECT 1 FROM job j WHERE j.id = job_id AND j.user_id = current_setting('app.user_id', true))
  );
