package catalog

import (
	"context"

	"github.com/Somnusochi/material-dispatch-server/internal/platform/business"
)

func (s *Service) Map(ctx context.Context, specID string) ([]business.Row, error) {
	if specID != "" {
		if _, err := business.ID(specID); err != nil {
			return nil, err
		}
	}
	return mapLocations.Rows(ctx, s.pool, specID)
}

func (s *Service) MapMaterials(ctx context.Context, locationID string, page business.Page) (business.List, error) {
	id, err := business.ID(locationID)
	if err != nil {
		return business.List{}, err
	}
	if _, err = activeMapLocation.One(ctx, s.pool, id); err != nil {
		return business.List{}, err
	}
	return listMapMaterials.Paginate(ctx, s.pool, []any{id, page.Keyword, page.SpecID}, page)
}

func (s *Service) MapSummary(ctx context.Context, specID, keyword, locationID string) (business.Row, error) {
	if specID != "" {
		if _, err := business.ID(specID); err != nil {
			return nil, err
		}
	}
	if len([]rune(keyword)) > 255 {
		return nil, business.Invalid("地点搜索关键词不能超过255字")
	}
	if locationID != "" {
		id, err := business.ID(locationID)
		if err != nil {
			return nil, err
		}
		if _, err = activeMapLocation.One(ctx, s.pool, id); err != nil {
			return nil, err
		}
	}
	return mapSummary.One(ctx, s.pool, specID, keyword, locationID)
}
