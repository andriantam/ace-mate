package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/andriantam/ace-mate/internal/model"
)

// ───────── profile ─────────

type ProfileRow struct {
	Data model.Profile `json:"data"`
}

func (d *DB) GetProfile(ctx context.Context, userID string) (*ProfileRow, error) {
	s, err := d.sealerFor(ctx, userID)
	if err != nil {
		return nil, err
	}
	var out *ProfileRow
	err = d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		var r ProfileRow
		var enc []byte
		var plain *model.Profile // rows written before encryption, until the startup pass
		err := tx.QueryRow(ctx, `SELECT data_enc, data FROM profile WHERE user_id=$1`, userID).Scan(&enc, &plain)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if ok, err := s.openJSON("profile.data", userID, enc, &r.Data); err != nil {
			return err
		} else if !ok {
			if plain == nil { // a legacy row with no data reads as "no profile"
				return nil
			}
			r.Data = *plain
		}
		out = &r
		return nil
	})
	return out, err
}

func (d *DB) UpsertProfile(ctx context.Context, userID string, p model.Profile) error {
	s, err := d.sealerFor(ctx, userID)
	if err != nil {
		return err
	}
	enc, err := s.json("profile.data", userID, p)
	if err != nil {
		return err
	}
	return d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO profile (user_id,data,data_enc) VALUES ($1,NULL,$2)
			ON CONFLICT (user_id) DO UPDATE SET data=NULL, data_enc=EXCLUDED.data_enc, updated_at=now()`, userID, enc)
		return err
	})
}
