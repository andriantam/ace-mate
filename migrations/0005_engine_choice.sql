-- Remember which model the user picked, so a queued task, a retry or a "fill the gaps"
-- run uses the same model (and the same privacy label) the user confirmed.
-- Additive only: existing rows keep working and read '' = "the site default".
ALTER TABLE task ADD COLUMN IF NOT EXISTS engine text NOT NULL DEFAULT '';
ALTER TABLE job  ADD COLUMN IF NOT EXISTS engine text NOT NULL DEFAULT '';
