package db

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/andriantam/ace-mate/internal/model"
)

// Document is something the user added to build their profile. Only the AI's extraction
// is stored, never the original file.
type Document struct {
	ID           string          `json:"id"`
	Kind         string          `json:"kind"`
	Name         string          `json:"name"`
	URL          string          `json:"url"`
	Status       string          `json:"status"`
	Error        string          `json:"error"`
	Extracted    *model.Profile  `json:"extracted"`
	Verification json.RawMessage `json:"verification"`
	CreatedAt    time.Time       `json:"createdAt"`
}

func (d *DB) InsertDocument(ctx context.Context, userID, kind, name, url string) (string, error) {
	var id string
	err := d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `INSERT INTO document (user_id,kind,name,url) VALUES ($1,$2,$3,NULLIF($4,'')) RETURNING id`,
			userID, kind, name, url).Scan(&id)
	})
	return id, err
}

// FinishDocument stores the extraction (and the cross-check report, if any) of a document.
// name, when not empty, replaces the document's label (a link gets its page title).
func (d *DB) FinishDocument(ctx context.Context, userID, id, name string, p model.Profile, verification any) error {
	return d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE document SET status='ready', error=NULL, extracted=$3, verification=$4,
			name=CASE WHEN $5 <> '' THEN $5 ELSE name END, updated_at=now()
			WHERE id=$1 AND user_id=$2`, id, userID, p, verification, name)
		return err
	})
}

func (d *DB) FailDocument(ctx context.Context, userID, id, msg string) error {
	return d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE document SET status='failed', error=$3, updated_at=now() WHERE id=$1 AND user_id=$2`, id, userID, msg)
		return err
	})
}

func (d *DB) ListDocuments(ctx context.Context, userID string) ([]Document, error) {
	out := []Document{}
	err := d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id,kind,name,COALESCE(url,''),status,COALESCE(error,''),extracted,verification,created_at
			FROM document WHERE user_id=$1 ORDER BY created_at`, userID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var doc Document
			if err := rows.Scan(&doc.ID, &doc.Kind, &doc.Name, &doc.URL, &doc.Status, &doc.Error, &doc.Extracted, &doc.Verification, &doc.CreatedAt); err != nil {
				return err
			}
			out = append(out, doc)
		}
		return rows.Err()
	})
	return out, err
}

func (d *DB) GetDocument(ctx context.Context, userID, id string) (*Document, error) {
	list, err := d.ListDocuments(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == id {
			return &list[i], nil
		}
	}
	return nil, ErrNotFound
}

func (d *DB) DeleteDocument(ctx context.Context, userID, id string) error {
	return d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		t, err := tx.Exec(ctx, `DELETE FROM document WHERE id=$1 AND user_id=$2`, id, userID)
		if err == nil && t.RowsAffected() == 0 {
			return ErrNotFound
		}
		return err
	})
}

// DeleteProfile removes the merged profile (when the user removes every document).
func (d *DB) DeleteProfile(ctx context.Context, userID string) error {
	return d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM profile WHERE user_id=$1`, userID)
		return err
	})
}
