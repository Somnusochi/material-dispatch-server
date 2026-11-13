package auth

import (
	"context"
	"regexp"
	"strconv"

	"github.com/Somnusochi/material-dispatch-server/internal/audit"
	"github.com/Somnusochi/material-dispatch-server/internal/platform/business"
	"github.com/jackc/pgx/v5"
)

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{3,64}$`)

type User struct {
	ID       int64  `json:"id,string"`
	Username string `json:"username"`
	RealName string `json:"real_name"`
	Phone    string `json:"phone"`
	Role     string `json:"role"`
	Status   int    `json:"status"`
}
type UserInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
	RealName string `json:"real_name"`
	Phone    string `json:"phone"`
	Role     string `json:"role"`
	Status   int    `json:"status"`
}

func validateUser(input UserInput, create bool) error {
	if create && !usernamePattern.MatchString(input.Username) {
		return business.Invalid("用户名需为3至64位字母、数字、点、短横线或下划线")
	}
	if err := business.RequireText(input.RealName, "姓名", 64); err != nil {
		return err
	}
	if input.Role != "admin" && input.Role != "dispatcher" && input.Role != "inspector" {
		return business.Invalid("无效角色")
	}
	if input.Status != 0 && input.Status != 1 {
		return business.Invalid("无效用户状态")
	}
	if len(input.Phone) > 20 {
		return business.Invalid("手机号过长")
	}
	return nil
}
func (s *Service) CreateInitialAdmin(ctx context.Context, username, password, name string) (User, error) {
	return s.CreateUser(ctx, business.Actor{Username: "local-bootstrap"}, UserInput{Username: username, Password: password, RealName: name, Role: "admin", Status: 1})
}
func (s *Service) CreateUser(ctx context.Context, actor business.Actor, input UserInput) (User, error) {
	if err := validateUser(input, true); err != nil {
		return User{}, err
	}
	hash, err := hashPassword(input.Password)
	if err != nil {
		return User{}, err
	}
	var user User
	err = business.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		user, err = scanUser(insertUser.Row(ctx, tx, input.Username, hash, input.RealName, input.Phone, input.Role, input.Status))
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, actor, "user", "create", strconv.FormatInt(user.ID, 10), "创建用户", nil, user)
	})
	return user, err
}
func (s *Service) UpdateUser(ctx context.Context, actor business.Actor, id int64, input UserInput) (User, error) {
	if err := validateUser(input, false); err != nil {
		return User{}, err
	}
	var user User
	var passwordHash string
	var err error
	if input.Password != "" {
		passwordHash, err = hashPassword(input.Password)
		if err != nil {
			return user, err
		}
	}
	err = business.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := lockUserAdministration.Exec(ctx, tx); err != nil {
			return err
		}
		old, err := scanUser(lockUser.Row(ctx, tx, id))
		if err != nil {
			return err
		}
		if old.Role == "admin" && old.Status == 1 && (input.Role != "admin" || input.Status != 1) {
			var total int
			if err = enabledAdminCount.Row(ctx, tx).Scan(&total); err != nil {
				return err
			}
			if total <= 1 {
				return business.Conflict("至少保留一名启用的管理员")
			}
		}
		if actor.ID == id && (input.Status != 1 || input.Role != "admin") {
			return business.Conflict("不能停用或降级当前管理员")
		}
		user, err = scanUser(updateUser.Row(ctx, tx, input.RealName, input.Phone, input.Role, input.Status, passwordHash, id))
		if err != nil {
			return err
		}
		if old.Role != input.Role || input.Status == 0 || passwordHash != "" {
			if _, err = deleteUserSessions.Exec(ctx, tx, id); err != nil {
				return err
			}
		}
		return audit.Record(ctx, tx, actor, "user", "update", strconv.FormatInt(id, 10), "更新用户", old, user)
	})
	return user, err
}
func (s *Service) List(ctx context.Context, p business.Page) (business.List, error) {
	return listUsers.Paginate(ctx, s.pool, []any{p.Keyword}, p)
}

// Contacts exposes only identifiers and display names for assigning a location manager.
func (s *Service) Contacts(ctx context.Context, p business.Page) (business.List, error) {
	return listContacts.Paginate(ctx, s.pool, []any{p.Keyword}, p)
}
