package service

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
)

// RunScheduler periodically activates due scheduled campaigns until ctx is cancelled.
func RunScheduler(ctx context.Context, log *logrus.Logger, svc *Service, period time.Duration) {
	if period <= 0 {
		period = 30 * time.Second
	}
	t := time.NewTicker(period)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			ids, err := svc.ActivateDueCampaigns(context.Background())
			if err != nil {
				if log != nil {
					log.WithError(err).Warn("campaign scheduler tick failed")
				}
				continue
			}
			if len(ids) > 0 && log != nil {
				log.WithField("campaign_ids", ids).Info("activated scheduled campaigns")
			}
		}
	}
}
