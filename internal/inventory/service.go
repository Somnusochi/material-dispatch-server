package inventory

import (
	"context"
	"sort"
	"strconv"

	"github.com/Somnusochi/material-dispatch-server/internal/audit"
	"github.com/Somnusochi/material-dispatch-server/internal/catalog"
	"github.com/Somnusochi/material-dispatch-server/internal/platform/business"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

type Stock struct {
	ID         int64 `json:"id,string"`
	LocationID int64 `json:"location_id,string"`
	SpecID     int64 `json:"spec_id,string"`
	Normal     int   `json:"normal_qty"`
	Damaged    int   `json:"damaged_qty"`
	Transit    int   `json:"in_transit_qty"`
	Version    int   `json:"version"`
}

// Lock creates absent balances and acquires locks in a stable location order.
func Lock(ctx context.Context, tx pgx.Tx, locations []int64, spec int64) (map[int64]Stock, error) {
	locations = append([]int64(nil), locations...)
	sort.Slice(locations, func(i, j int) bool { return locations[i] < locations[j] })
	result := map[int64]Stock{}
	for _, id := range locations {
		if _, ok := result[id]; ok {
			continue
		}
		if _, err := ensureStock.Exec(ctx, tx, id, spec); err != nil {
			return nil, err
		}
		var stock Stock
		if err := lockStock.Row(ctx, tx, id, spec).Scan(&stock.ID, &stock.LocationID, &stock.SpecID, &stock.Normal, &stock.Damaged, &stock.Transit, &stock.Version); err != nil {
			return nil, err
		}
		result[id] = stock
	}
	return result, nil
}
func Save(ctx context.Context, tx pgx.Tx, actor business.Actor, stock Stock) error {
	if stock.Normal < 0 || stock.Damaged < 0 || stock.Transit < 0 {
		return business.Conflict("库存不足或库存状态已变化")
	}
	_, err := saveStock.Exec(ctx, tx, stock.Normal, stock.Damaged, stock.Transit, actor.ID, stock.ID)
	return err
}

type AdjustInput struct {
	LocationID string `json:"location_id"`
	SpecID     string `json:"spec_id"`
	Normal     int    `json:"normal_qty"`
	Damaged    int    `json:"damaged_qty"`
	Version    int    `json:"version"`
	Reason     string `json:"reason"`
}

func (s *Service) Adjust(ctx context.Context, actor business.Actor, in AdjustInput) (Stock, error) {
	var zero Stock
	loc, err := business.ID(in.LocationID)
	if err != nil {
		return zero, err
	}
	spec, err := business.ID(in.SpecID)
	if err != nil {
		return zero, err
	}
	if in.Normal < 0 || in.Damaged < 0 || in.Normal > 100000000 || in.Damaged > 100000000 || in.Version < 0 {
		return zero, business.Invalid("库存数量或版本无效")
	}
	if err = business.RequireText(in.Reason, "调整原因", 500); err != nil {
		return zero, err
	}
	var result Stock
	err = business.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := catalog.Active(ctx, tx, []int64{loc}, spec); err != nil {
			return err
		}
		// A stable per-balance lock also serializes the expected-version=0 creation case.
		if _, err := lockBalance.Exec(ctx, tx, "stock:"+in.LocationID+":"+in.SpecID); err != nil {
			return err
		}
		var exists bool
		if err := stockExists.Row(ctx, tx, loc, spec).Scan(&exists); err != nil {
			return err
		}
		balances, err := Lock(ctx, tx, []int64{loc}, spec)
		if err != nil {
			return err
		}
		before := balances[loc]
		if (!exists && in.Version != 0) || (exists && in.Version != before.Version) {
			return business.Conflict("库存已变化，请刷新后重试")
		}
		result = before
		result.Normal = in.Normal
		result.Damaged = in.Damaged
		// Do not allow adjustments to remove damage already awaiting disposition.
		var reserved int
		if err = reservedDamage.Row(ctx, tx, loc, spec).Scan(&reserved); err != nil {
			return err
		}
		if result.Damaged < reserved {
			return business.Conflict("损毁库存不能低于待处置损毁单数量")
		}
		if err = Save(ctx, tx, actor, result); err != nil {
			return err
		}
		result.Version++
		return audit.Record(ctx, tx, actor, "inventory", "adjust", strconv.FormatInt(result.ID, 10), "库存调整", before, map[string]any{"inventory": result, "reason": in.Reason})
	})
	return result, err
}
func (s *Service) List(ctx context.Context, p business.Page) (business.List, error) {
	return listStock.Paginate(ctx, s.pool, []any{p.LocationID, p.SpecID, p.Keyword}, p)
}
