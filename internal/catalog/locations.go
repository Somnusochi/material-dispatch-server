package catalog

import (
	"context"
	"math"
	"strconv"
	"strings"

	"github.com/Somnusochi/material-dispatch-server/internal/audit"
	"github.com/Somnusochi/material-dispatch-server/internal/platform/business"
	"github.com/jackc/pgx/v5"
)

type LocationInput struct {
	ManagerID    string   `json:"manager_id"`
	Name         string   `json:"name"`
	Code         string   `json:"code"`
	Type         string   `json:"loc_type"`
	Address      string   `json:"address"`
	Longitude    *float64 `json:"longitude"`
	Latitude     *float64 `json:"latitude"`
	ContactPhone string   `json:"contact_phone"`
	Status       int      `json:"status"`
	Remark       string   `json:"remark"`
}

func (s *Service) Locations(ctx context.Context, p business.Page) (business.List, error) {
	return listLocations.Paginate(ctx, s.pool, []any{p.Keyword, p.Status}, p)
}
func (s *Service) SaveLocation(ctx context.Context, actor business.Actor, id int64, in LocationInput) (business.Row, error) {
	for _, v := range []struct {
		value, name string
		max         int
	}{{in.Name, "地点名称", 128}, {in.Type, "地点类型", 32}} {
		if err := business.RequireText(v.value, v.name, v.max); err != nil {
			return nil, err
		}
	}
	if in.Type != "warehouse" && in.Type != "site" && in.Type != "yard" {
		return nil, business.Invalid("地点类型无效")
	}
	if len([]rune(in.Code)) > 64 || len([]rune(in.Address)) > 255 || len(in.ContactPhone) > 20 || len([]rune(in.Remark)) > 2000 {
		return nil, business.Invalid("地点字段过长")
	}
	if in.Longitude == nil || in.Latitude == nil || math.IsNaN(*in.Longitude) || math.IsNaN(*in.Latitude) || *in.Longitude < -180 || *in.Longitude > 180 || *in.Latitude < -90 || *in.Latitude > 90 {
		return nil, business.Invalid("请选择有效的 WGS84 地图位置")
	}
	if in.Status != 0 && in.Status != 1 {
		return nil, business.Invalid("地点状态无效")
	}
	var managerID any
	if in.ManagerID != "" {
		id, err := business.ID(in.ManagerID)
		if err != nil {
			return nil, err
		}
		managerID = id
	}
	var result, before business.Row
	err := business.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		action := "create"
		if id > 0 {
			action = "update"
			before, err = lockLocation.One(ctx, tx, id)
			if err != nil {
				return err
			}
			if in.Status == 0 {
				var used bool
				err = locationInUse.Row(ctx, tx, id).Scan(&used)
				if err != nil {
					return err
				}
				if used {
					return business.Conflict("地点仍有库存或未完成调度，不能停用")
				}
			}
			_, err = updateLocation.Exec(ctx, tx, strings.TrimSpace(in.Name), in.Code, in.Type, in.Address, *in.Longitude, *in.Latitude, in.ContactPhone, in.Status, in.Remark, managerID, id)
		} else {
			err = insertLocation.Row(ctx, tx, strings.TrimSpace(in.Name), in.Code, in.Type, in.Address, *in.Longitude, *in.Latitude, in.ContactPhone, in.Status, in.Remark, managerID).Scan(&id)
		}
		if err != nil {
			return err
		}
		result, err = getLocation.One(ctx, tx, id)
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, actor, "location", action, strconv.FormatInt(id, 10), "维护地点", before, result)
	})
	return result, err
}
