-- Per-user data keys for application-level encryption.
--
-- Each user has one random data key. It is stored only wrapped (encrypted) by a master key
-- that lives outside the database (ENCRYPTION_KEYS), so this table alone reveals nothing.
-- Deleting a user's row makes everything encrypted under their key unreadable.
--
-- Like the identity tables, it is not granted to the restricted app role: only the server's
-- own key code reads it, never a query running as a user.

CREATE TABLE IF NOT EXISTS user_key (
  user_id      text PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  kek_version  int   NOT NULL CHECK (kek_version > 0),  -- which master key wrapped it
  wrapped      bytea NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now(),
  rotated_at   timestamptz
);
