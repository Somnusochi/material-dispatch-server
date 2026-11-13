package damage

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/Somnusochi/material-dispatch-server/internal/audit"
	"github.com/Somnusochi/material-dispatch-server/internal/catalog"
	"github.com/Somnusochi/material-dispatch-server/internal/dispatch"
	"github.com/Somnusochi/material-dispatch-server/internal/inventory"
	"github.com/Somnusochi/material-dispatch-server/internal/platform/business"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func (s *Service) List(ctx context.Context, p business.Page) (business.List, error) {
	return listReports.Paginate(ctx, s.pool, []any{p.Status, p.Keyword, p.LocationID, p.SpecID}, p)
}
func get(ctx context.Context, q business.Queryer, id int64) (business.Row, error) {
	return findReport.One(ctx, q, id)
}
func (s *Service) Get(ctx context.Context, id int64) (business.Row, error) {
	return get(ctx, s.pool, id)
}

type CreateInput struct {
	Location    string   `json:"location_id"`
	Spec        string   `json:"spec_id"`
	Qty         int      `json:"damaged_qty"`
	Level       string   `json:"damage_level"`
	Description string   `json:"description"`
	Images      []string `json:"image_urls"`
}

func (s *Service) Create(ctx context.Context, actor business.Actor, in CreateInput) (business.Row, error) {
	loc, err := business.ID(in.Location)
	if err != nil {
		return nil, err
	}
	spec, err := business.ID(in.Spec)
	if err != nil {
		return nil, err
	}
	if in.Qty <= 0 || in.Qty > 100000000 {
		return nil, business.Invalid("损毁数量必须为正整数")
	}
	// 等级仅兼容旧客户端与既有表约束，不再要求录入，也不参与库存处置。
	if in.Level == "" {
		in.Level = "moderate"
	}
	if in.Level != "minor" && in.Level != "moderate" && in.Level != "scrapped" {
		return nil, business.Invalid("损毁等级无效")
	}
	if err = business.RequireText(in.Description, "损毁说明", 2000); err != nil {
		return nil, err
	}
	var result business.Row
	err = business.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := catalog.Active(ctx, tx, []int64{loc}, spec); err != nil {
			return err
		}
		if err := catalog.CheckImages(ctx, tx, in.Images); err != nil {
			return err
		}
		images := in.Images
		if images == nil {
			images = []string{}
		}
		raw, _ := json.Marshal(images)
		var id int64
		if err := insertReport.Row(ctx, tx, dispatch.Number("SH"), loc, spec, in.Qty, in.Level, in.Description, raw, actor.ID).Scan(&id); err != nil {
			return err
		}
		var err error
		result, err = get(ctx, tx, id)
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, actor, "damage", "create", strconv.FormatInt(id, 10), "上报损毁", nil, result)
	})
	return result, err
}
func (s *Service) Transition(ctx context.Context, actor business.Actor, id int64, action, remark string) (business.Row, error) {
	if len([]rune(remark)) > 2000 {
		return nil, business.Invalid("处理意见过长")
	}
	var result business.Row
	err := business.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		var loc, spec int64
		var qty int
		var status string
		if err := lockReport.Row(ctx, tx, id).Scan(&loc, &spec, &qty, &status); err != nil {
			return err
		}
		before, err := get(ctx, tx, id)
		if err != nil {
			return err
		}
		next := ""
		switch action {
		case "confirm":
			if status != "pending" {
				return business.Conflict("只能确认待审核损毁单")
			}
			next = "confirmed"
		case "reject":
			if status != "pending" {
				return business.Conflict("只能驳回待审核损毁单")
			}
			next = "rejected"
		case "repair":
			if status != "confirmed" {
				return business.Conflict("只能修复已确认损毁单")
			}
			next = "repaired"
		case "scrap":
			if status != "confirmed" {
				return business.Conflict("只能报废已确认损毁单")
			}
			next = "scrapped"
		default:
			return business.NotFound()
		}
		if action != "reject" {
			stocks, err := inventory.Lock(ctx, tx, []int64{loc}, spec)
			if err != nil {
				return err
			}
			stock := stocks[loc]
			switch action {
			case "confirm":
				stock.Normal -= qty
				stock.Damaged += qty
			case "repair":
				stock.Normal += qty
				stock.Damaged -= qty
			case "scrap":
				stock.Damaged -= qty
			}
			if err = inventory.Save(ctx, tx, actor, stock); err != nil {
				return err
			}
		}
		if _, err = updateReport.Exec(ctx, tx, next, actor.ID, remark, id); err != nil {
			return err
		}
		result, err = get(ctx, tx, id)
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, actor, "damage", action, strconv.FormatInt(id, 10), "损毁处置", before, result)
	})
	return result, err
}
