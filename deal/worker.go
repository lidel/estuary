package deal

import (
	"context"
	"time"

	"github.com/application-research/estuary/model"
	"github.com/application-research/estuary/util"
	"gorm.io/gorm"
)

func (m *manager) runWorkers(ctx context.Context) {
	m.log.Infof("starting up deal workers")

	go m.runDealBackFillWorker(ctx)

	go m.runDealCheckWorker(ctx)

	go m.runDealWorker(ctx)

	m.log.Infof("spun up deal workers")
}

func (m *manager) runDealBackFillWorker(ctx context.Context) {
	timer := time.NewTicker(m.cfg.WorkerIntervals.DealInterval)
	for {
		select {
		case <-timer.C:
			tracker, err := m.getQueueTracker()
			if err != nil {
				m.log.Warnf("failed to get deal queue tracker - %s", err)
				continue
			}

			if tracker.LastContID > tracker.StopAt {
				m.log.Info("deal queue backfill is done")
				return
			}

			m.log.Debugf("trying to backfill deal queue starting from content: %d", tracker.LastContID)

			var contents []*util.Content
			if err := m.db.Where("size > ? and size < ? and active", m.cfg.Content.MinSize, m.cfg.Content.MaxSize).Order("id asc").FindInBatches(&contents, 2000, func(tx *gorm.DB, batch int) error {
				m.log.Debugf("trying to backfill deal queue for total of %d contents", len(contents))

				for _, c := range contents {
					if err := m.backfillQueue(c, tracker); err != nil {
						return err
					}
				}
				return nil
			}).Error; err != nil {
				m.log.Warnf("failed to backfill deal queue - %s", err)
			}
		}
	}
}

func (m *manager) runDealWorker(ctx context.Context) {
	timer := time.NewTicker(m.cfg.WorkerIntervals.DealInterval)
	for {
		select {
		case <-timer.C:
			var tasks []*model.DealQueue
			if err := m.db.Where("commp_done and can_deal and deal_next_attempt_at < ?", time.Now().UTC()).Order("id asc").FindInBatches(&tasks, 2000, func(tx *gorm.DB, batch int) error {
				m.log.Debugf("trying to make deal for total of %d contents", len(tasks))
				for _, t := range tasks {
					m.log.Infow("making more deals for content", "content", t.ContID, "newDeals", t.DealCount)
					if err := m.makeDealsForContent(ctx, t.ContID, t.DealCount); err != nil {
						m.log.Errorf("failed to make more deals for cont: %d - %s", t.ContID, err)
						m.dealQueueMgr.DealFailed(t.ContID)
						continue
					}
					m.dealQueueMgr.DealFailed(t.ContID)
				}
				return nil
			}).Error; err != nil {
				m.log.Errorf("", err)
			}
		}
	}
}

func (m *manager) runDealCheckWorker(ctx context.Context) {
	timer := time.NewTicker(m.cfg.WorkerIntervals.DealInterval)
	for {
		select {
		case <-timer.C:
			var tasks []*model.DealQueue
			if err := m.db.Where("commp_done and not can_deal and deal_check_next_attempt_at < ?", time.Now().UTC()).Order("id asc").FindInBatches(&tasks, 2000, func(tx *gorm.DB, batch int) error {
				m.log.Debugf("trying to check %d deals", len(tasks))
				for _, t := range tasks {
					if err := m.checkContentDeals(ctx, t.ContID); err != nil {
						m.log.Warnf("failed to check cont %d deals - %s", t.ContID, err)
						m.dealQueueMgr.DealCheckFailed(t.ContID)
						continue
					}
				}
				return nil
			}).Error; err != nil {
				m.log.Warnf("failed to check deals - %s", err)
			}
		}
	}
}

func (m *manager) getQueueTracker() (*model.DealQueueTracker, error) {
	var trackers []*model.DealQueueTracker
	if err := m.db.Find(&trackers).Error; err != nil {
		return nil, err
	}

	if len(trackers) == 0 {
		var contents []*util.Content
		if err := m.db.Order("id asc").Limit(1).Find(&contents).Error; err != nil {
			return nil, err
		}

		trk := &model.DealQueueTracker{LastContID: 0, StopAt: contents[0].ID}
		if err := m.db.Create(&trk).Error; err != nil {
			return nil, err
		}
		return trk, nil
	}
	return trackers[0], nil
}

func (m *manager) backfillQueue(cont *util.Content, tracker *model.DealQueueTracker) error {
	m.log.Debugf("trying to backfill deal queue for content %d", cont.ID)

	if err := m.dealQueueMgr.QueueContent(cont); err != nil {
		return err
	}
	return m.db.Model(model.DealQueueTracker{}).Where("id = ?", tracker.ID).UpdateColumn("last_cont_id", cont.ID).Error
}
