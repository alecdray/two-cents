package auth

import (
	"context"
	"database/sql"
	"errors"

	"github.com/alecdray/two-cents/src/internal/core/db/sqlc"
)

// Repo is the auth module's data access layer — the only file in package auth
// that imports core/db/sqlc. Its methods take and return this package's types,
// never sqlc.* shapes.
type Repo struct {
	q *sqlc.Queries
}

// NewRepo binds a Repo to the given Queries.
func NewRepo(q *sqlc.Queries) *Repo {
	return &Repo{q: q}
}

// GetCredential returns the single stored credential. The bool is false (with a
// nil error) when no credential has been seeded yet.
func (r *Repo) GetCredential(ctx context.Context) (credential, bool, error) {
	model, err := r.q.GetUser(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return credential{}, false, nil
		}
		return credential{}, false, err
	}
	return credential{id: model.ID, username: model.Username, hash: model.PasswordHash}, true, nil
}

// UpsertCredential writes the single credential row, inserting it on first
// write and replacing the username + hash thereafter.
func (r *Repo) UpsertCredential(ctx context.Context, username, hash string) error {
	_, err := r.q.UpsertUser(ctx, sqlc.UpsertUserParams{
		ID:           userID,
		Username:     username,
		PasswordHash: hash,
	})
	return err
}
