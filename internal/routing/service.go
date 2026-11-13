package routing

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/Somnusochi/material-dispatch-server/internal/platform/business"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Options struct{ RefreshHour, DailyLimit int }
type Service struct {
	repo                    repository
	provider                Provider
	refreshHour, dailyLimit int
	interval                time.Duration
}
type DestinationList struct {
	business.List
	PendingCount int64 `json:"pending_count"`
}

var shanghai = time.FixedZone("Asia/Shanghai", 8*60*60)

func NewService(pool *pgxpool.Pool, provider Provider, options Options) *Service {
	if options.DailyLimit <= 0 {
		options.DailyLimit = 1000
	}
	return &Service{repo: repository{pool}, provider: provider, refreshHour: options.RefreshHour, dailyLimit: options.DailyLimit, interval: 500 * time.Millisecond}
}

func (s *Service) Destinations(ctx context.Context, source string, p business.Page) (DestinationList, error) {
	id, err := business.ID(source)
	if err != nil {
		return DestinationList{}, err
	}
	p.Keyword = strings.TrimSpace(p.Keyword)
	if p.Page < 1 || p.Size < 1 || p.Size > 100 || len(p.Keyword) > 256 {
		return DestinationList{}, business.Invalid("目的地查询条件无效")
	}
	if err = activeSource(ctx, s.repo.pool, id); err != nil {
		return DestinationList{}, err
	}
	if s.provider != nil {
		if err = s.repo.enqueue(ctx, id); err != nil {
			return DestinationList{}, err
		}
	}
	return s.repo.list(ctx, id, p, s.provider != nil)
}

func nextRefresh(now time.Time, hour int) time.Time {
	now = now.In(shanghai)
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, shanghai)
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// Run catches up after downtime, processes newly requested routes, and refreshes
// saved pairs at their persisted daily next_attempt_at (02:00 by default).
func (s *Service) Run(ctx context.Context, logger *slog.Logger) {
	if s.provider == nil {
		return
	}
	for {
		if _, err := s.RefreshBatch(ctx, 20); err != nil && ctx.Err() == nil {
			logger.Error("route cache refresh failed")
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (s *Service) RefreshBatch(ctx context.Context, limit int) (int, error) {
	if s.provider == nil {
		return 0, nil
	}
	conn, err := acquireWorker(ctx, s.repo.pool)
	if err != nil || conn == nil {
		return 0, err
	}
	defer releaseWorker(conn)
	jobs, err := dueJobs(ctx, conn, limit)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, j := range jobs {
		if err = ctx.Err(); err != nil {
			return processed, err
		}
		now := time.Now().In(shanghai)
		allowed, err := reserveBudget(ctx, conn, now.Format("2006-01-02"), s.dailyLimit)
		if err != nil {
			return processed, err
		}
		if !allowed {
			tomorrow := time.Date(now.Year(), now.Month(), now.Day()+1, s.refreshHour, 0, 0, 0, shanghai)
			return processed, postponeDue(ctx, conn, tomorrow, "budget_exhausted")
		}
		callCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		meters, providerErr := s.provider.Distance(callCtx, j.from, j.to)
		cancel()
		if ctx.Err() != nil {
			return processed, ctx.Err()
		}
		next := nextRefresh(time.Now(), s.refreshHour)
		code := ""
		var distance *int64
		if providerErr == nil {
			distance = &meters
		} else {
			code = "provider_unavailable"
			next = time.Now().Add(15 * time.Minute)
		}
		if errors.Is(providerErr, errProviderDenied) {
			code = "provider_denied"
			next = time.Now().Add(time.Hour)
		}
		if err = saveResult(ctx, conn, j, distance, next, code); err != nil {
			return processed, err
		}
		processed++
		if errors.Is(providerErr, errProviderDenied) {
			return processed, postponeDue(ctx, conn, next, code)
		}
		if s.interval > 0 {
			timer := time.NewTimer(s.interval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return processed, ctx.Err()
			case <-timer.C:
			}
		}
	}
	return processed, nil
}
