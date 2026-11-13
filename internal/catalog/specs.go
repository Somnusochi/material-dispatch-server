package catalog

import (
	"context"
	"crypto/rand"
	"math"
	"strconv"
	"strings"

	"github.com/Somnusochi/material-dispatch-server/internal/audit"
	"github.com/Somnusochi/material-dispatch-server/internal/platform/business"
	"github.com/jackc/pgx/v5"
)

type SpecInput struct {
	Code       *string  `json:"spec_code"`
	Name       string   `json:"name"`
	Category   *string  `json:"category"`
	Weight     *float64 `json:"unit_weight_kg"`
	Dimensions *string  `json:"dimensions"`
	Unit       string   `json:"unit"`
	ImageURL   *string  `json:"image_url"`
	Remark     *string  `json:"remark"`
}

func (s *Service) Specs(ctx context.Context, p business.Page) (business.List, error) {
	return listSpecs.Paginate(ctx, s.pool, []any{p.Keyword}, p)
}
func (s *Service) SaveSpec(ctx context.Context, actor business.Actor, id int64, in SpecInput) (business.Row, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Unit = strings.TrimSpace(in.Unit)
	for _, v := range []struct {
		value *string
		name  string
		max   int
	}{{in.Code, "规格编码", 64}, {&in.Name, "名称", 128}, {in.Category, "分类", 64}, {&in.Unit, "单位", 16}} {
		if v.value == nil {
			continue
		}
		if err := business.RequireText(*v.value, v.name, v.max); err != nil {
			return nil, err
		}
	}
	if in.Weight != nil && (math.IsNaN(*in.Weight) || math.IsInf(*in.Weight, 0) || *in.Weight < 0 || *in.Weight > 99999999.99) {
		return nil, business.Invalid("规格单重无效")
	}
	if (in.Dimensions != nil && len([]rune(*in.Dimensions)) > 128) || (in.Remark != nil && len([]rune(*in.Remark)) > 2000) {
		return nil, business.Invalid("规格参数无效")
	}
	if id == 0 && in.Code == nil {
		code := "MAT-" + rand.Text()
		in.Code = &code
	}
	var result, before business.Row
	err := business.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		if in.ImageURL != nil && *in.ImageURL != "" {
			if err := CheckImages(ctx, tx, []string{*in.ImageURL}); err != nil {
				return err
			}
		}
		var err error
		action := "create"
		if id > 0 {
			action = "update"
			before, err = lockSpec.One(ctx, tx, id)
			if err != nil {
				return err
			}
			_, err = updateSpec.Exec(ctx, tx, in.Code, in.Name, in.Category, in.Weight, in.Dimensions, in.Unit, in.ImageURL, in.Remark, id)
		} else {
			err = insertSpec.Row(ctx, tx, in.Code, in.Name, in.Category, in.Weight, in.Dimensions, in.Unit, in.ImageURL, in.Remark).Scan(&id)
		}
		if err != nil {
			return err
		}
		result, err = getSpec.One(ctx, tx, id)
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, actor, "spec", action, strconv.FormatInt(id, 10), "维护物料规格", before, result)
	})
	return result, err
}
