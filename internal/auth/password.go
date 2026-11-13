package auth

import (
	"context"
	"strconv"

	"github.com/Somnusochi/material-dispatch-server/internal/audit"
	"github.com/Somnusochi/material-dispatch-server/internal/platform/business"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

const PasswordCost = 12

func ValidatePassword(password string) error {
	if len(password) < 10 || len(password) > 72 {
		return business.Invalid("密码长度需为10至72字节")
	}
	return nil
}
func hashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), PasswordCost)
	return string(hash), err
}
func (s *Service) ChangePassword(ctx context.Context, actor business.Actor, current, next string) error {
	hash, err := hashPassword(next)
	if err != nil {
		return err
	}
	return business.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		var old string
		if err := lockedPassword.Row(ctx, tx, actor.ID).Scan(&old); err != nil {
			return err
		}
		if bcrypt.CompareHashAndPassword([]byte(old), []byte(current)) != nil {
			return business.Invalid("当前密码不正确")
		}
		if _, err := updatePassword.Exec(ctx, tx, hash, actor.ID); err != nil {
			return err
		}
		if _, err := deleteUserSessions.Exec(ctx, tx, actor.ID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, actor, "auth", "change_password", strconv.FormatInt(actor.ID, 10), "修改密码并撤销会话", nil, nil)
	})
}
