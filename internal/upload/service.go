package upload

import (
	"bytes"
	"context"
	"crypto/rand"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/Somnusochi/material-dispatch-server/internal/audit"
	"github.com/Somnusochi/material-dispatch-server/internal/platform/business"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "golang.org/x/image/webp"
)

const MaxBytes = 5 << 20

type Service struct {
	pool *pgxpool.Pool
	dir  string
}

func NewService(pool *pgxpool.Pool, dir string) *Service { return &Service{pool: pool, dir: dir} }

type File struct {
	ID          string `json:"id"`
	URL         string `json:"url"`
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Size        int    `json:"size"`
}

func (s *Service) Save(ctx context.Context, actor business.Actor, name string, data []byte) (File, error) {
	var result File
	if len(data) == 0 || len(data) > MaxBytes {
		return result, business.Invalid("图片大小需在5MB以内")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 20000 || cfg.Height > 20000 || int64(cfg.Width)*int64(cfg.Height) > 20000000 {
		return result, business.Invalid("图片无效或分辨率过大")
	}
	mime := map[string]string{"jpeg": "image/jpeg", "png": "image/png", "webp": "image/webp"}[format]
	if mime == "" {
		return result, business.Invalid("仅支持 JPG、PNG、WebP 图片")
	}
	if _, _, err = image.Decode(bytes.NewReader(data)); err != nil {
		return result, business.Invalid("图片文件损坏")
	}
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if len([]rune(name)) > 200 {
		name = string([]rune(name)[:200])
	}
	id := rand.Text()
	key := id + "." + format
	path := filepath.Join(s.dir, key)
	if err = os.MkdirAll(s.dir, 0700); err != nil {
		return result, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return result, err
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(path)
		if writeErr != nil {
			return result, writeErr
		}
		return result, closeErr
	}
	result = File{ID: id, URL: "/api/v1/uploads/" + id, Name: name, ContentType: mime, Size: len(data)}
	err = business.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := insertFile.Exec(ctx, tx, id, actor.ID, key, name, mime, len(data)); err != nil {
			return err
		}
		return audit.Record(ctx, tx, actor, "upload", "create", id, "上传业务图片", nil, result)
	})
	if err != nil {
		_ = os.Remove(path)
	}
	return result, err
}
func (s *Service) Read(ctx context.Context, id string) (string, string, error) {
	if len(id) > 64 || strings.ContainsAny(id, "./\\") {
		return "", "", business.NotFound()
	}
	var key, mime string
	err := loadFile.Row(ctx, s.pool, id).Scan(&key, &mime)
	if err != nil {
		return "", "", business.Normalize(err)
	}
	if filepath.Base(key) != key {
		return "", "", business.NotFound()
	}
	return filepath.Join(s.dir, key), mime, nil
}
