-- The personal details a sign-in provider shares (email, name, photo link, birthday) are
-- stored encrypted with the user's own data key, as one sealed JSON value. The server
-- encrypts existing rows once at startup and clears the plaintext columns.
--
-- Left in plaintext: the id, locale, whether the email is verified, consent records and
-- timestamps. Sign-in matches on (provider, subject) in accounts, never on email, so
-- nothing needs to search these values.

ALTER TABLE users ADD COLUMN IF NOT EXISTS personal_enc bytea;
