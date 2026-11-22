package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/andriantam/ace-mate/internal/model"
)

type Export struct {
	ExportedAt time.Time     `json:"exportedAt"`
	Account    ExportAccount `json:"account"`
	Profile    *ProfileRow   `json:"profile"`
	Documents  []Document    `json:"documents"`
	Jobs       []ExportedJob `json:"jobs"`
}

// ExportAccount is everything we hold about the user's identity and consent.
type ExportAccount struct {
	Email           string     `json:"email"`
	Name            string     `json:"name"`
	GivenName       string     `json:"givenName"`
	FamilyName      string     `json:"familyName"`
	Image           string     `json:"image"`
	Locale          string     `json:"locale"`
	Birthday        string     `json:"birthday"`
	EmailVerified   bool       `json:"emailVerified"`
	SignInProviders []string   `json:"signInProviders"`
	TermsVersion    string     `json:"termsVersion"`
	TermsAcceptedAt *time.Time `json:"termsAcceptedAt"`
	MarketingOptIn  bool       `json:"marketingOptIn"`
	CreatedAt       time.Time  `json:"createdAt"`
}

type ExportedJob struct {
	ID        string         `json:"id"`
	URL       string         `json:"url"`
	Title     string         `json:"title"`
	Company   string         `json:"company"`
	Notes     string         `json:"notes"`
	Status    string         `json:"status"`
	CreatedAt time.Time      `json:"createdAt"`
	Journey   *model.Journey `json:"journey"`
	People    model.People   `json:"people"`
	Progress  model.Progress `json:"progress"`
}

func (d *DB) ExportAll(ctx context.Context, userID string) (*Export, error) {
	p, err := d.GetProfile(ctx, userID)
	if err != nil {
		return nil, err
	}
	list, err := d.ListJobs(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := &Export{ExportedAt: time.Now().UTC(), Profile: p, Jobs: []ExportedJob{}}
	if out.Documents, err = d.ListDocuments(ctx, userID); err != nil {
		return nil, err
	}
	a := &out.Account
	var enc []byte
	var plain personal
	if err := d.Pool.QueryRow(ctx, `SELECT personal_enc,email,name,given_name,family_name,image,birthday,locale,email_verified,
		terms_version,terms_accepted_at,marketing_opt_in,created_at FROM users WHERE id=$1`, userID).Scan(
		&enc, &plain.Email, &plain.Name, &plain.GivenName, &plain.FamilyName, &plain.Image, &plain.Birthday, &a.Locale, &a.EmailVerified,
		&a.TermsVersion, &a.TermsAcceptedAt, &a.MarketingOptIn, &a.CreatedAt); err != nil {
		return nil, err
	}
	s, err := d.sealerFor(ctx, userID)
	if err != nil {
		return nil, err
	}
	pd, err := openPersonal(s, enc, plain)
	if err != nil {
		return nil, err
	}
	a.Email, a.Name, a.GivenName, a.FamilyName, a.Image, a.Birthday = pd.Email, pd.Name, pd.GivenName, pd.FamilyName, pd.Image, pd.Birthday
	rows, err := d.Pool.Query(ctx, `SELECT provider FROM accounts WHERE user_id=$1 ORDER BY provider`, userID)
	if err != nil {
		return nil, err
	}
	a.SignInProviders, err = pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	for _, s := range list {
		j, err := d.GetJob(ctx, userID, s.ID)
		if err != nil {
			return nil, err
		}
		out.Jobs = append(out.Jobs, ExportedJob{ID: j.ID, URL: j.URL, Title: j.Title, Company: j.Company, Notes: j.Notes, Status: j.Status,
			CreatedAt: j.CreatedAt, Journey: j.Journey, People: j.People, Progress: j.Progress})
	}
	return out, nil
}
