package postgres

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"linha/server/internal/domain"
)

func (s *Store) RefreshSession(ctx context.Context, hash string, renew func(domain.BrowserSession) (domain.BrowserSession, error)) (out domain.BrowserSession, err error) {
	var rejected error
	err = s.tx(ctx, func(tx pgx.Tx) error {
		current := domain.BrowserSession{}
		e := tx.QueryRow(ctx, "SELECT hash,claims,csrf,expires_at,auth_expires_at,refresh_cipher FROM linha_browser_sessions WHERE hash=$1 AND expires_at>clock_timestamp() FOR UPDATE", hash).Scan(&current.Hash, &current.Claims, &current.CSRF, &current.ExpiresAt, &current.AuthExpiresAt, &current.RefreshCipher)
		if e != nil {
			return missing(e)
		}
		next, e := renew(current)
		if e != nil {
			var auth *domain.Error
			if errors.As(e, &auth) && (auth.Status == 401 || auth.Status == 403) {
				rejected = e
				_, e = tx.Exec(ctx, "DELETE FROM linha_browser_sessions WHERE hash=$1", hash)
			}
			return e
		}
		if next.Hash != current.Hash || !next.ExpiresAt.Equal(current.ExpiresAt) {
			return domain.Bad("refresh cannot change session identity or absolute expiry")
		}
		_, e = tx.Exec(ctx, "UPDATE linha_browser_sessions SET claims=$2,auth_expires_at=$3,refresh_cipher=$4 WHERE hash=$1", hash, next.Claims, next.AuthExpiresAt, next.RefreshCipher)
		out = next
		return e
	})
	if err == nil && rejected != nil {
		err = rejected
	}
	return
}
