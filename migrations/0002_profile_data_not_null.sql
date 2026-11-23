-- Databases created by an earlier pre-release schema may have profile.data nullable
-- (CREATE TABLE IF NOT EXISTS in 0001 does not tighten an existing column).
-- Non-destructive: keep every row (and its user_id mapping), replace NULL with an empty
-- profile, then enforce NOT NULL. The profile is derived from document.extracted and is
-- refilled the next time a document finishes or the profile is rebuilt.
UPDATE profile SET data = '{}'::jsonb, updated_at = now() WHERE data IS NULL;
ALTER TABLE profile ALTER COLUMN data SET NOT NULL;
