package db

import "errors"

// Data access layer. Rules:
//  1. The user id always comes from the server-side session, never from request input.
//  2. Every query on user data runs through WithUser, so Postgres enforces ownership.
//  3. Queries still filter by user_id (defence in depth).

var ErrNotFound = errors.New("not found")

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
