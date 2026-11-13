// admin is an explicit local operator command; it never seeds a default account.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Somnusochi/material-dispatch-server/internal/auth"
	"github.com/Somnusochi/material-dispatch-server/internal/config"
	"github.com/Somnusochi/material-dispatch-server/internal/platform/postgres"
	"github.com/joho/godotenv"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(args []string, in io.Reader, out io.Writer) error {
	flags := flag.NewFlagSet("admin", flag.ContinueOnError)
	flags.SetOutput(out)
	username := flags.String("username", "", "明确指定管理员用户名")
	name := flags.String("name", "", "管理员姓名")
	stdin := flags.Bool("password-stdin", false, "从标准输入读取密码，禁止命令行明文密码")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *username == "" || *name == "" || !*stdin || flags.NArg() != 0 {
		return errors.New("请指定 --username、--name 和 --password-stdin")
	}
	raw, err := io.ReadAll(io.LimitReader(in, 74))
	if err != nil {
		return errors.New("无法读取密码")
	}
	password := strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r")
	if err = auth.ValidatePassword(password); err != nil {
		return err
	}
	if err = godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("无法读取 .env")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, cfg.DatabaseURL, 2)
	if err != nil {
		return err
	}
	defer pool.Close()
	user, err := auth.NewService(pool).CreateInitialAdmin(ctx, *username, password, *name)
	if err != nil {
		return errors.New("创建管理员失败，请检查参数、用户名是否重复以及数据库迁移状态")
	}
	fmt.Fprintf(out, "管理员已创建：%s（ID %d）\n", user.Username, user.ID)
	return nil
}
