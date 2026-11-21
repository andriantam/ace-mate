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
	var out *ProfileRow
	err := d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		var r ProfileRow
		err := tx.QueryRow(ctx, `SELECT data FROM profile WHERE user_id=$1`, userID).Scan(&r.Data)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		out = &r
		return nil
	})
	return out, err
}

func (d *DB) UpsertProfile(ctx context.Context, userID string, p model.Profile) error {
	return d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO profile (user_id,data) VALUES ($1,$2)
			ON CONFLICT (user_id) DO UPDATE SET data=EXCLUDED.data, updated_at=now()`, userID, p)
		return err
	})
}
