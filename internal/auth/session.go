package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/Somnusochi/material-dispatch-server/internal/audit"
	"github.com/Somnusochi/material-dispatch-server/internal/platform/business"
	"github.com/jackc/pgx/v5"
)

const AccessDuration = 24 * time.Hour
const RefreshDuration = 7 * 24 * time.Hour

func HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

type Session struct {
	User             User      `json:"user"`
	CSRF             string    `json:"csrf_token"`
	AccessExpiresAt  time.Time `json:"access_expires_at"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
}
type Tokens struct{ Access, Refresh string }

func (s *Service) Authenticate(ctx context.Context, token string) (Session, error) {
	var session Session
	if len(token) < 24 || len(token) > 128 {
		return session, business.Unauthorized()
	}
	u := &session.User
	err := sessionUser.Row(ctx, s.pool, HashToken(token)).Scan(&u.ID, &u.Username, &u.RealName, &u.Phone, &u.Role, &u.Status, &session.CSRF, &session.AccessExpiresAt, &session.RefreshExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = business.Unauthorized()
	}
	return session, err
}

// issueTokens keeps the family CSRF and absolute expiry stable across rotations.
func issueTokens(ctx context.Context, tx pgx.Tx, family string, session *Session) (Tokens, error) {
	tokens := Tokens{Access: rand.Text() + rand.Text(), Refresh: rand.Text() + rand.Text()}
	session.AccessExpiresAt = time.Now().Add(AccessDuration)
	if session.AccessExpiresAt.After(session.RefreshExpiresAt) {
		session.AccessExpiresAt = session.RefreshExpiresAt
	}
	if _, err := insertSession.Exec(ctx, tx, HashToken(tokens.Access), session.User.ID, session.AccessExpiresAt, family); err != nil {
		return Tokens{}, err
	}
	_, err := insertRefresh.Exec(ctx, tx, HashToken(tokens.Refresh), family)
	return tokens, err
}
func (s *Service) Refresh(ctx context.Context, token string, actor business.Actor) (Session, Tokens, error) {
	var session Session
	var tokens Tokens
	if len(token) < 24 || len(token) > 128 {
		return session, tokens, business.Unauthorized()
	}
	denied := false
	err := business.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		var userID int64
		err := refreshUserID.Row(ctx, tx, HashToken(token)).Scan(&userID)
		if errors.Is(err, pgx.ErrNoRows) {
			denied = true
			return nil
		}
		if err != nil {
			return err
		}
		// User first, then family: same lock order as password/role/disable changes.
		session.User, err = scanUser(lockUser.Row(ctx, tx, userID))
		if errors.Is(err, pgx.ErrNoRows) {
			denied = true
			return nil
		}
		if err != nil {
			return err
		}
		var family string
		var used *time.Time
		err = lockedRefresh.Row(ctx, tx, HashToken(token)).Scan(&family, &session.CSRF, &session.RefreshExpiresAt, &used)
		if errors.Is(err, pgx.ErrNoRows) {
			denied = true
			return nil
		}
		if err != nil {
			return err
		}
		if used != nil || !session.RefreshExpiresAt.After(time.Now()) || session.User.Status != 1 {
			denied = true
			if _, err = revokeFamily.Exec(ctx, tx, family); err != nil {
				return err
			}
			if used != nil {
				actor.ID = session.User.ID
				actor.Username = session.User.Username
				return audit.Record(ctx, tx, actor, "auth", "refresh_reuse", family, "检测到续签令牌重用，撤销会话", nil, nil)
			}
			return nil
		}
		if _, err = consumeRefresh.Exec(ctx, tx, HashToken(token)); err != nil {
			return err
		}
		tokens, err = issueTokens(ctx, tx, family, &session)
		return err
	})
	if err != nil {
		return Session{}, Tokens{}, err
	}
	// Revocation must commit before returning 401, rather than rolling it back.
	if denied {
		return Session{}, Tokens{}, business.Unauthorized()
	}
	return session, tokens, nil
}
