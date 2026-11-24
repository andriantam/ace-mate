-- Prep shared across a user's job applications.
--
-- Several journeys often ask for the same thing: the same exercise in the same kind of role,
-- or the same generic HR question ("What is your biggest weakness?"). Each one is kept once
-- per user as a shared_item, and shared_link ties it to the places it appears in each
-- journey. Ticking a shared task ticks it in every linked journey; an HR question keeps the
-- user's own answer once, for the FAQ on their profile.
--
-- Additive only: no existing table or row is changed or removed. Existing jobs are linked
-- by a background pass after start-up (job.shared_synced_at is NULL until then), which
-- carries their ticked tasks over.
--
-- Safe to re-run: a database that already has these tables from a pre-release build of this
-- feature (recorded as 0002_shared_prep.sql) is brought up to date without losing rows.
--
-- Encryption (see 0003_encrypt_columns.sql): the label and the user's answer are personal
-- content, so they are stored sealed with the owner's data key in label_enc and answer_enc.
-- The plaintext label and answer columns stay empty; the server encrypts any plaintext left
-- by the pre-release build at start-up. The key is a generic slug such as
-- "hr/greatest-weakness" and stays readable, because new journeys are matched against it.

CREATE TABLE IF NOT EXISTS shared_item (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind        text NOT NULL CHECK (kind IN ('task', 'hr')),
  -- A canonical slug. Tasks carry their track, so the same exercise for a frontend and a
  -- backend role stays separate: "frontend/js-project-on-github", "backend/js-project-on-github".
  -- Generic HR questions share one key across all jobs: "hr/greatest-weakness".
  key         text NOT NULL,
  label       text NOT NULL DEFAULT '',        -- unused: see label_enc
  done        boolean NOT NULL DEFAULT false, -- tasks: ticked in every linked journey
  answer      text NOT NULL DEFAULT '',        -- unused: see answer_enc
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (user_id, kind, key)
);

-- Where a shared item appears. ref is "<nodeId>:<taskIndex>" for a task (the same string as
-- its progress item) and "<nodeId>:q<index>" for an HR question.
CREATE TABLE IF NOT EXISTS shared_link (
  job_id   uuid NOT NULL REFERENCES job(id) ON DELETE CASCADE,
  ref      text NOT NULL,
  item_id  uuid NOT NULL REFERENCES shared_item(id) ON DELETE CASCADE,
  user_id  text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  PRIMARY KEY (job_id, ref)
);
CREATE INDEX IF NOT EXISTS shared_link_item_idx ON shared_link (item_id);
CREATE INDEX IF NOT EXISTS shared_link_user_idx ON shared_link (user_id);

ALTER TABLE shared_item ADD COLUMN IF NOT EXISTS label_enc  bytea;
ALTER TABLE shared_item ADD COLUMN IF NOT EXISTS answer_enc bytea;
ALTER TABLE shared_item ALTER COLUMN label SET DEFAULT '';

-- When the job's journey was last compared with the user's other jobs. NULL: not yet.
ALTER TABLE job ADD COLUMN IF NOT EXISTS shared_synced_at timestamptz;

DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app_user') THEN
    GRANT SELECT, INSERT, UPDATE, DELETE ON shared_item, shared_link TO app_user;
  END IF;
END $$;

ALTER TABLE shared_item ENABLE ROW LEVEL SECURITY;
ALTER TABLE shared_item FORCE  ROW LEVEL SECURITY;
ALTER TABLE shared_link ENABLE ROW LEVEL SECURITY;
ALTER TABLE shared_link FORCE  ROW LEVEL SECURITY;

DROP POLICY IF EXISTS shared_item_owner ON shared_item;
CREATE POLICY shared_item_owner ON shared_item
  USING (user_id = current_setting('app.user_id', true))
  WITH CHECK (user_id = current_setting('app.user_id', true));

-- A link may only join the user's own job to the user's own item.
DROP POLICY IF EXISTS shared_link_owner ON shared_link;
CREATE POLICY shared_link_owner ON shared_link
  USING (user_id = current_setting('app.user_id', true))
  WITH CHECK (
    user_id = current_setting('app.user_id', true)
    AND EXISTS (SELECT 1 FROM job j WHERE j.id = job_id AND j.user_id = current_setting('app.user_id', true))
    AND EXISTS (SELECT 1 FROM shared_item s WHERE s.id = item_id AND s.user_id = current_setting('app.user_id', true))
  );
