-- Encrypted copies of every column that holds a user's personal content.
--
-- Each *_enc column holds the value sealed with the owner's data key (AES-256-GCM, see
-- internal/crypt), bound to its table, column, owner and row, so a value copied to another
-- row or another user does not decrypt. The server encrypts existing plaintext into these
-- columns once at startup, then clears the plaintext column. From then on only the
-- encrypted copy is written.
--
-- Left in plaintext on purpose: ids, statuses, timestamps, error messages, the job title
-- and company (shown and sorted in the jobs list), the document kind, and the progress
-- messages of background tasks (short-lived: deleted an hour after the task finishes).

ALTER TABLE profile ADD COLUMN IF NOT EXISTS data_enc bytea;
ALTER TABLE profile ALTER COLUMN data DROP NOT NULL;

ALTER TABLE document ADD COLUMN IF NOT EXISTS name_enc         bytea;
ALTER TABLE document ADD COLUMN IF NOT EXISTS url_enc          bytea;
ALTER TABLE document ADD COLUMN IF NOT EXISTS extracted_enc    bytea;
ALTER TABLE document ADD COLUMN IF NOT EXISTS verification_enc bytea;

ALTER TABLE job ADD COLUMN IF NOT EXISTS url_enc     bytea;
ALTER TABLE job ADD COLUMN IF NOT EXISTS notes_enc   bytea;
ALTER TABLE job ADD COLUMN IF NOT EXISTS jd_enc      bytea;
ALTER TABLE job ADD COLUMN IF NOT EXISTS journey_enc bytea;
ALTER TABLE job ADD COLUMN IF NOT EXISTS people_enc  bytea;

-- Uploaded files (resumes) waiting to be read.
ALTER TABLE task ADD COLUMN IF NOT EXISTS input_enc bytea;
