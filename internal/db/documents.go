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
	s, err := d.sealerFor(ctx, userID)
	if err != nil {
		return "", err
	}
	id := newUUID()
	nameEnc, err := s.text("document.name", id, name)
	if err != nil {
		return "", err
	}
	urlEnc, err := s.text("document.url", id, url)
	if err != nil {
		return "", err
	}
	err = d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO document (id,user_id,kind,name_enc,url_enc) VALUES ($1,$2,$3,$4,$5)`,
			id, userID, kind, nameEnc, urlEnc)
		return err
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

// FinishDocument stores the extraction (and the cross-check report, if any) of a document.
// name, when not empty, replaces the document's label (a link gets its page title).
func (d *DB) FinishDocument(ctx context.Context, userID, id, name string, p model.Profile, verification any) error {
	s, err := d.sealerFor(ctx, userID)
	if err != nil {
		return err
	}
	extracted, err := s.json("document.extracted", id, p)
	if err != nil {
		return err
	}
	verif, err := s.json("document.verification", id, verification)
	if err != nil {
		return err
	}
	nameEnc, err := s.text("document.name", id, name) // NULL when empty: keep the current name
	if err != nil {
		return err
	}
	return d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE document SET status='ready', error=NULL, extracted=NULL, verification=NULL,
			extracted_enc=$3, verification_enc=$4, name_enc=COALESCE($5, name_enc), updated_at=now()
			WHERE id=$1 AND user_id=$2`, id, userID, extracted, verif, nameEnc)
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
	s, err := d.sealerFor(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := []Document{}
	err = d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		// The plaintext columns are only read for rows written before encryption, until the
		// startup pass encrypts them.
		rows, err := tx.Query(ctx, `SELECT id,kind,status,COALESCE(error,''),created_at,
			name_enc,url_enc,extracted_enc,verification_enc,name,COALESCE(url,''),extracted,verification
			FROM document WHERE user_id=$1 ORDER BY created_at`, userID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var doc Document
			var nameEnc, urlEnc, extEnc, verEnc []byte
			if err := rows.Scan(&doc.ID, &doc.Kind, &doc.Status, &doc.Error, &doc.CreatedAt,
				&nameEnc, &urlEnc, &extEnc, &verEnc, &doc.Name, &doc.URL, &doc.Extracted, &doc.Verification); err != nil {
				return err
			}
			if err := openDocument(s, &doc, nameEnc, urlEnc, extEnc, verEnc); err != nil {
				return err
			}
			out = append(out, doc)
		}
		return rows.Err()
	})
	return out, err
}

// openDocument fills doc from its encrypted columns, where they are set.
func openDocument(s sealer, doc *Document, nameEnc, urlEnc, extEnc, verEnc []byte) error {
	if nameEnc != nil {
		v, err := s.openText("document.name", doc.ID, nameEnc)
		if err != nil {
			return err
		}
		doc.Name = v
	}
	if urlEnc != nil {
		v, err := s.openText("document.url", doc.ID, urlEnc)
		if err != nil {
			return err
		}
		doc.URL = v
	}
	if extEnc != nil {
		var p model.Profile
		if _, err := s.openJSON("document.extracted", doc.ID, extEnc, &p); err != nil {
			return err
		}
		doc.Extracted = &p
	}
	if verEnc != nil {
		v, err := s.openBytes("document.verification", doc.ID, verEnc)
		if err != nil {
			return err
		}
		doc.Verification = v
	}
	return nil
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
