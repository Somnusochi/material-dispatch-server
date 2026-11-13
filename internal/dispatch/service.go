package dispatch

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"strconv"
	"time"

	"github.com/Somnusochi/material-dispatch-server/internal/audit"
	"github.com/Somnusochi/material-dispatch-server/internal/catalog"
	"github.com/Somnusochi/material-dispatch-server/internal/inventory"
	"github.com/Somnusochi/material-dispatch-server/internal/platform/business"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }
func Number(prefix string) string {
	return prefix + time.Now().UTC().Format("20060102150405") + rand.Text()[:8]
}

func (s *Service) List(ctx context.Context, p business.Page) (business.List, error) {
	return listOrders.Paginate(ctx, s.pool, []any{p.Status, p.Keyword, p.LocationID, p.SpecID}, p)
}
func (s *Service) Get(ctx context.Context, id int64) (business.Row, error) {
	return get(ctx, s.pool, id)
}
func get(ctx context.Context, q business.Queryer, id int64) (business.Row, error) {
	return findOrder.One(ctx, q, id)
}

type CreateInput struct {
	From     string `json:"from_location_id"`
	To       string `json:"to_location_id"`
	Spec     string `json:"spec_id"`
	Qty      int    `json:"dispatch_qty"`
	Priority int    `json:"priority"`
	Remark   string `json:"remark"`
}

func (s *Service) Create(ctx context.Context, actor business.Actor, in CreateInput) (business.Row, error) {
	from, err := business.ID(in.From)
	if err != nil {
		return nil, err
	}
	to, err := business.ID(in.To)
	if err != nil {
		return nil, err
	}
	spec, err := business.ID(in.Spec)
	if err != nil {
		return nil, err
	}
	if from == to || in.Qty <= 0 || in.Qty > 100000000 || in.Priority < 1 || in.Priority > 3 || len([]rune(in.Remark)) > 2000 {
		return nil, business.Invalid("起终点、数量或优先级无效")
	}
	var result business.Row
	err = business.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := catalog.Active(ctx, tx, []int64{from, to}, spec); err != nil {
			return err
		}
		var id int64
		if err := insertOrder.Row(ctx, tx, Number("DD"), from, to, spec, in.Qty, in.Priority, actor.ID, in.Remark).Scan(&id); err != nil {
			return err
		}
		var err error
		result, err = get(ctx, tx, id)
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, actor, "dispatch", "create", strconv.FormatInt(id, 10), "创建调度单", nil, result)
	})
	return result, err
}

type ActionInput struct {
	Normal      int      `json:"normal_qty"`
	Damaged     int      `json:"damaged_qty"`
	Description string   `json:"description"`
	Images      []string `json:"image_urls"`
	Remark      string   `json:"remark"`
}

func (s *Service) Transition(ctx context.Context, actor business.Actor, id int64, action string, in ActionInput) (business.Row, error) {
	if len([]rune(in.Description)) > 2000 || len([]rune(in.Remark)) > 2000 {
		return nil, business.Invalid("调度字段过长")
	}
	var result business.Row
	err := business.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		var from, to, spec int64
		var qty int
		var status string
		if err := lockOrder.Row(ctx, tx, id).Scan(&from, &to, &spec, &qty, &status); err != nil {
			return err
		}
		before, err := get(ctx, tx, id)
		if err != nil {
			return err
		}
		switch action {
		case "cancel":
			if status != "pending" {
				return business.Conflict("只能取消待调度的单据")
			}
			if _, err = cancelOrder.Exec(ctx, tx, id); err != nil {
				return err
			}
		case "depart":
			if status != "pending" {
				return business.Conflict("当前单据无法调度")
			}
			stocks, err := inventory.Lock(ctx, tx, []int64{from, to}, spec)
			if err != nil {
				return err
			}
			a, b := stocks[from], stocks[to]
			a.Normal -= qty
			b.Transit += qty
			if err = inventory.Save(ctx, tx, actor, a); err != nil {
				return err
			}
			if err = inventory.Save(ctx, tx, actor, b); err != nil {
				return err
			}
			if _, err = departOrder.Exec(ctx, tx, actor.ID, id); err != nil {
				return err
			}
		case "receive":
			if status != "in_transit" {
				return business.Conflict("当前调度单不能签收")
			}
			if in.Normal < 0 || in.Damaged < 0 || in.Normal > qty || in.Damaged > qty || in.Normal+in.Damaged != qty {
				return business.Invalid("完好数量与损毁数量之和必须等于调拨数量")
			}
			if in.Damaged > 0 {
				if err = business.RequireText(in.Description, "损毁说明", 2000); err != nil {
					return err
				}
			}
			if err = catalog.CheckImages(ctx, tx, in.Images); err != nil {
				return err
			}
			stocks, err := inventory.Lock(ctx, tx, []int64{to}, spec)
			if err != nil {
				return err
			}
			stock := stocks[to]
			stock.Transit -= qty
			stock.Normal += in.Normal
			stock.Damaged += in.Damaged
			if err = inventory.Save(ctx, tx, actor, stock); err != nil {
				return err
			}
			if _, err = receiveOrder.Exec(ctx, tx, in.Normal, actor.ID, id); err != nil {
				return err
			}
			if in.Damaged > 0 {
				images := in.Images
				if images == nil {
					images = []string{}
				}
				raw, _ := json.Marshal(images)
				var reportID int64
				if err = insertTransitDamage.Row(ctx, tx, Number("SH"), to, spec, id, in.Damaged, in.Description, raw, actor.ID).Scan(&reportID); err != nil {
					return err
				}
				if err = audit.Record(ctx, tx, actor, "damage", "receive", strconv.FormatInt(reportID, 10), "签收损毁入库", nil, map[string]any{"dispatch_order_id": strconv.FormatInt(id, 10), "damaged_qty": in.Damaged}); err != nil {
					return err
				}
			}
		default:
			return business.NotFound()
		}
		result, err = get(ctx, tx, id)
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, actor, "dispatch", action, strconv.FormatInt(id, 10), "调度状态变更", before, result)
	})
	return result, err
}
