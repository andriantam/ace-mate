-- Databases created by an earlier pre-release schema may have profile.data nullable
-- (CREATE TABLE IF NOT EXISTS in 0001 does not tighten an existing column).
-- Drop NULL rows (no profile) and enforce NOT NULL so reads can't hit NULL.
DELETE FROM profile WHERE data IS NULL;
ALTER TABLE profile ALTER COLUMN data SET NOT NULL;
