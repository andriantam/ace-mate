package db

// Users, sign-in identities, consent and sessions.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type User struct {
	ID, Email, Name, Image string
	GivenName, FamilyName  string
	Locale, Birthday       string
	EmailVerified          bool
	TermsVersion           string
	TermsAcceptedAt        *time.Time
	MarketingOptIn         bool
}

// Identity is what a sign-in provider tells us about the user. Only the fields the
// provider shared are filled.
type Identity struct {
	Provider, Subject     string
	Email, Name, Image    string
	GivenName, FamilyName string
	Locale, Birthday      string
	EmailVerified         bool
}

// ───────── identity (no RLS: these tables are not granted to the app role) ─────────

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// UpsertOAuthUser finds or creates the user for a provider identity.
func (d *DB) UpsertOAuthUser(ctx context.Context, provider, subject, email, name, image string) (string, error) {
	return d.UpsertIdentity(ctx, Identity{Provider: provider, Subject: subject, Email: email, Name: name, Image: image})
}

// UpsertIdentity finds or creates the user for a provider identity and refreshes the
// profile details the provider shared. Identity is (provider, subject): the same email
// at two providers is two accounts, so a provider can never take over an account it
// doesn't own.
func (d *DB) UpsertIdentity(ctx context.Context, in Identity) (string, error) {
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var id string
	var dek []byte // the user's data key
	err = tx.QueryRow(ctx, `SELECT user_id FROM accounts WHERE provider=$1 AND subject=$2`, in.Provider, in.Subject).Scan(&id)
	switch {
	case err == nil:
		if dek, err = d.UserKey(ctx, id); err != nil {
			return "", err
		}
	case errors.Is(err, pgx.ErrNoRows):
		id = newID()
		if _, err := tx.Exec(ctx, `INSERT INTO users (id) VALUES ($1)`, id); err != nil {
			return "", err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO accounts (provider,subject,user_id) VALUES ($1,$2,$3)`, in.Provider, in.Subject, id); err != nil {
			return "", err
		}
		// The user's data key is created with the account, in the same transaction.
		if dek, err = d.insertUserKey(ctx, tx, id); err != nil {
			return "", err
		}
	default:
		return "", err
	}
	s := sealer{dek: dek, userID: id}
	p := personal{Email: in.Email, Name: in.Name, Image: in.Image, GivenName: in.GivenName, FamilyName: in.FamilyName, Birthday: in.Birthday}
	// The birthday is only overwritten when the provider shared one this time, so turning
	// the feature off later doesn't silently erase it (deleting the account does).
	if p.Birthday == "" {
		var enc []byte
		var plain string
		if err := tx.QueryRow(ctx, `SELECT personal_enc, birthday FROM users WHERE id=$1`, id).Scan(&enc, &plain); err != nil {
			return "", err
		}
		old, err := openPersonal(s, enc, personal{Birthday: plain})
		if err != nil {
			return "", err
		}
		p.Birthday = old.Birthday
	}
	enc, err := s.json("users.personal", id, p)
	if err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET personal_enc=$2, locale=$3, email_verified=$4,
		email='', name='', image='', given_name='', family_name='', birthday='' WHERE id=$1`,
		id, enc, in.Locale, in.EmailVerified); err != nil {
		return "", err
	}
	return id, tx.Commit(ctx)
}

// personal is what a sign-in provider shares about the user, stored encrypted in
// users.personal_enc.
type personal struct {
	Email      string `json:"email,omitempty"`
	Name       string `json:"name,omitempty"`
	Image      string `json:"image,omitempty"`
	GivenName  string `json:"givenName,omitempty"`
	FamilyName string `json:"familyName,omitempty"`
	Birthday   string `json:"birthday,omitempty"`
}

// openPersonal decrypts users.personal_enc. plain holds the plaintext columns, used for a
// row not yet encrypted.
func openPersonal(s sealer, enc []byte, plain personal) (personal, error) {
	var p personal
	ok, err := s.openJSON("users.personal", s.userID, enc, &p)
	if err != nil || !ok {
		return plain, err
	}
	return p, nil
}

// AcceptTerms records acceptance of a terms version and the separate marketing choice.
func (d *DB) AcceptTerms(ctx context.Context, userID, version string, marketing bool) error {
	_, err := d.Pool.Exec(ctx, `UPDATE users SET terms_version=$2, terms_accepted_at=now(),
		marketing_opt_in=$3, marketing_updated_at=now() WHERE id=$1`, userID, version, marketing)
	return err
}

// SetMarketing changes the optional marketing consent.
func (d *DB) SetMarketing(ctx context.Context, userID string, optIn bool) error {
	_, err := d.Pool.Exec(ctx, `UPDATE users SET marketing_opt_in=$2, marketing_updated_at=now() WHERE id=$1`, userID, optIn)
	return err
}

const (
	SessionTTL   = 7 * 24 * time.Hour
	sessionSlide = 24 * time.Hour
)

func hashToken(t string) []byte { h := sha256.Sum256([]byte(t)); return h[:] }

func (d *DB) CreateSession(ctx context.Context, userID string) (string, time.Time, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", time.Time{}, err
	}
	tok := base64.RawURLEncoding.EncodeToString(b)
	exp := time.Now().Add(SessionTTL)
	_, err := d.Pool.Exec(ctx, `INSERT INTO sessions (token_hash,user_id,expires_at) VALUES ($1,$2,$3)`, hashToken(tok), userID, exp)
	return tok, exp, err
}

// UserBySession returns the signed-in user and slides the expiry forward.
func (d *DB) UserBySession(ctx context.Context, token string) (*User, error) {
	var u User
	var exp time.Time
	var enc []byte
	var plain personal
	err := d.Pool.QueryRow(ctx, `
		SELECT u.id,u.personal_enc,u.email,u.name,u.image,u.given_name,u.family_name,u.birthday,u.locale,u.email_verified,
		       u.terms_version,u.terms_accepted_at,u.marketing_opt_in,s.expires_at
		FROM sessions s JOIN users u ON u.id=s.user_id
		WHERE s.token_hash=$1 AND s.expires_at > now()`, hashToken(token)).Scan(&u.ID, &enc, &plain.Email, &plain.Name, &plain.Image,
		&plain.GivenName, &plain.FamilyName, &plain.Birthday, &u.Locale, &u.EmailVerified, &u.TermsVersion, &u.TermsAcceptedAt, &u.MarketingOptIn, &exp)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	s, err := d.sealerFor(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	p, err := openPersonal(s, enc, plain)
	if err != nil {
		return nil, err
	}
	u.Email, u.Name, u.Image, u.GivenName, u.FamilyName, u.Birthday = p.Email, p.Name, p.Image, p.GivenName, p.FamilyName, p.Birthday
	if time.Until(exp) < SessionTTL-sessionSlide {
		_, _ = d.Pool.Exec(ctx, `UPDATE sessions SET expires_at=$2 WHERE token_hash=$1`, hashToken(token), time.Now().Add(SessionTTL))
	}
	return &u, nil
}

func (d *DB) DeleteSession(ctx context.Context, token string) error {
	_, err := d.Pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash=$1`, hashToken(token))
	return err
}

// DeleteUser removes the user; profile, jobs, progress, sessions and accounts cascade.
// Their data key is deleted first, explicitly, and dropped from memory, so anything
// encrypted under it (including copies in backups) can no longer be decrypted.
func (d *DB) DeleteUser(ctx context.Context, userID string) error {
	defer d.keyCache.evict(userID)
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM user_key WHERE user_id=$1`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM users WHERE id=$1`, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (d *DB) PurgeExpiredSessions(ctx context.Context) {
	_, _ = d.Pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at < now()`)
}
