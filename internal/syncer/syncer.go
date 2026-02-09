package syncer

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/vultisig/verifier/plugin/tx_indexer"
	"github.com/vultisig/verifier/plugin/tx_indexer/pkg/rpc"

	"github.com/vultisig/app-developer/internal/db"
)

type TxSyncer struct {
	txIndexer *tx_indexer.Service
	db        *db.PostgresBackend
	logger    *logrus.Logger
	interval  time.Duration
}

func NewTxSyncer(
	txIndexer *tx_indexer.Service,
	database *db.PostgresBackend,
	logger *logrus.Logger,
	interval time.Duration,
) *TxSyncer {
	return &TxSyncer{
		txIndexer: txIndexer,
		db:        database,
		logger:    logger.WithField("pkg", "syncer").Logger,
		interval:  interval,
	}
}

func (s *TxSyncer) Run(ctx context.Context) {
	s.logger.Info("tx syncer started")
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("tx syncer stopped")
			return
		case <-ticker.C:
			s.sync(ctx)
		}
	}
}

func (s *TxSyncer) sync(ctx context.Context) {
	fees, err := s.db.GetSubmittedListingFees(ctx)
	if err != nil {
		s.logger.WithError(err).Error("failed to get submitted listing fees")
		return
	}

	for _, fee := range fees {
		txs, _, err := s.txIndexer.GetByPolicyID(ctx, fee.PolicyID, 0, 1)
		if err != nil {
			s.logger.WithError(err).WithField("policy_id", fee.PolicyID).Error("failed to get tx_indexer status")
			continue
		}

		if len(txs) == 0 {
			continue
		}

		tx := txs[0]
		if tx.StatusOnChain == nil {
			continue
		}

		switch *tx.StatusOnChain {
		case rpc.TxOnChainSuccess:
			blockNum := int64(0)
			err = s.db.MarkAsPaid(ctx, fee.PolicyID, blockNum, 1)
			if err != nil {
				s.logger.WithError(err).WithField("policy_id", fee.PolicyID).Error("failed to mark as paid")
				continue
			}
			s.logger.WithFields(logrus.Fields{
				"policy_id": fee.PolicyID,
				"tx_hash":   tx.TxHash,
			}).Info("listing fee paid")

		case rpc.TxOnChainFail:
			err = s.db.MarkAsFailed(ctx, fee.PolicyID, "transaction failed on-chain")
			if err != nil {
				s.logger.WithError(err).WithField("policy_id", fee.PolicyID).Error("failed to mark as failed")
				continue
			}
			s.logger.WithFields(logrus.Fields{
				"policy_id": fee.PolicyID,
				"tx_hash":   tx.TxHash,
			}).Warn("listing fee transaction failed")
		}

		if tx.Lost {
			err = s.db.MarkAsFailed(ctx, fee.PolicyID, "transaction lost")
			if err != nil {
				s.logger.WithError(err).WithField("policy_id", fee.PolicyID).Error("failed to mark as failed")
			}
		}
	}
}
