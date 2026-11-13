package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Somnusochi/material-dispatch-server/internal/audit"
	"github.com/Somnusochi/material-dispatch-server/internal/platform/business"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

func (s *Service) Login(ctx context.Context, username, password string, actor business.Actor) (Session, Tokens, error) {
	if len(username) > 64 || len(password) > 72 {
		return Session{}, Tokens{}, business.Unauthorized()
	}
	var user User
	var tokens Tokens
	var session Session
	var denied bool
	err := business.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		var hash string
		var attempts int
		var locked *time.Time
		err := loginUser.Row(ctx, tx, strings.TrimSpace(username)).Scan(&user.ID, &user.Username, &user.RealName, &user.Phone, &user.Role, &user.Status, &hash, &attempts, &locked)
		if errors.Is(err, pgx.ErrNoRows) {
			denied = true
			return nil
		}
		if err != nil {
			return err
		}
		if user.Status != 1 || (locked != nil && locked.After(time.Now())) {
			denied = true
			return nil
		}
		if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
			denied = true
			_, err = recordFailedLogin.Exec(ctx, tx, user.ID)
			return err
		}
		if _, err = recordSuccessfulLogin.Exec(ctx, tx, user.ID); err != nil {
			return err
		}

		family := rand.Text()
		session = Session{User: user, CSRF: rand.Text() + rand.Text(), RefreshExpiresAt: time.Now().Add(RefreshDuration)}
		if _, err = insertFamily.Exec(ctx, tx, family, user.ID, session.CSRF, session.RefreshExpiresAt); err != nil {
			return err
		}
		tokens, err = issueTokens(ctx, tx, family, &session)
		if err != nil {
			return err
		}
		actor.ID = user.ID
		actor.Username = user.Username
		return audit.Record(ctx, tx, actor, "auth", "login", strconv.FormatInt(user.ID, 10), "用户登录", nil, nil)
	})
	if err != nil {
		return Session{}, Tokens{}, err
	}
	if denied {
		return Session{}, Tokens{}, &business.Error{Status: 401, Code: "INVALID_CREDENTIALS", Message: "用户名或密码错误，或账户暂不可用"}
	}
	return session, tokens, nil
}
func (s *Service) Logout(ctx context.Context, token string, actor business.Actor) error {
	return business.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := deleteSession.Exec(ctx, tx, HashToken(token)); err != nil {
			return err
		}
		return audit.Record(ctx, tx, actor, "auth", "logout", strconv.FormatInt(actor.ID, 10), "退出登录", nil, nil)
	})
}
