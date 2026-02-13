package db

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/vultisig/app-developer/internal/db/sqlcgen"
)

type ListingFee struct {
	ID             uuid.UUID
	PolicyID       uuid.UUID
	PublicKey      string
	TargetPluginID string
	Amount         *big.Int
	Destination    string
	TxHash         *string
	BlockNumber    *int64
	Confirmations  int
	Status         string
	SubmittedAt    *time.Time
	PaidAt         *time.Time
	FailureReason  *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (p *PostgresBackend) CreateListingFee(ctx context.Context, fee ListingFee) error {
	err := p.queries.CreateListingFee(ctx, sqlcgen.CreateListingFeeParams{
		PolicyID:       fee.PolicyID,
		PublicKey:      fee.PublicKey,
		TargetPluginID: fee.TargetPluginID,
		Amount:         fee.Amount.String(),
		Destination:    fee.Destination,
		Status:         fee.Status,
	})
	if err != nil {
		return fmt.Errorf("failed to create listing fee: %w", err)
	}
	return nil
}

func (p *PostgresBackend) GetListingFeeByPolicyID(ctx context.Context, policyID uuid.UUID) (*ListingFee, error) {
	row, err := p.queries.GetListingFeeByPolicyID(ctx, policyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get listing fee: %w", err)
	}
	return toListingFee(row), nil
}

func (p *PostgresBackend) GetListingFeeByScope(ctx context.Context, publicKey, pluginID string) (*ListingFee, error) {
	row, err := p.queries.GetListingFeeByScope(ctx, sqlcgen.GetListingFeeByScopeParams{
		PublicKey:      publicKey,
		TargetPluginID: pluginID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get listing fee by scope: %w", err)
	}
	return toListingFee(row), nil
}

func (p *PostgresBackend) GetPendingListingFeeByScope(ctx context.Context, publicKey, pluginID string) (*ListingFee, error) {
	row, err := p.queries.GetPendingListingFeeByScope(ctx, sqlcgen.GetPendingListingFeeByScopeParams{
		PublicKey:      publicKey,
		TargetPluginID: pluginID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get pending listing fee by scope: %w", err)
	}
	return toListingFee(row), nil
}

func (p *PostgresBackend) GetPendingListingFees(ctx context.Context) ([]ListingFee, error) {
	rows, err := p.queries.GetPendingListingFees(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending listing fees: %w", err)
	}
	return toListingFees(rows), nil
}

func (p *PostgresBackend) GetSubmittedListingFees(ctx context.Context) ([]ListingFee, error) {
	rows, err := p.queries.GetSubmittedListingFees(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query submitted listing fees: %w", err)
	}
	return toListingFees(rows), nil
}

func (p *PostgresBackend) MarkAsSubmitted(ctx context.Context, policyID uuid.UUID, txHash string) error {
	err := p.queries.MarkAsSubmitted(ctx, sqlcgen.MarkAsSubmittedParams{
		PolicyID: policyID,
		TxHash:   &txHash,
	})
	if err != nil {
		return fmt.Errorf("failed to mark listing fee as submitted: %w", err)
	}
	return nil
}

func (p *PostgresBackend) MarkAsPaid(ctx context.Context, policyID uuid.UUID, blockNum int64, confirmations int) error {
	err := p.queries.MarkAsPaid(ctx, sqlcgen.MarkAsPaidParams{
		PolicyID:      policyID,
		BlockNumber:   &blockNum,
		Confirmations: int32(confirmations),
	})
	if err != nil {
		return fmt.Errorf("failed to mark listing fee as paid: %w", err)
	}
	return nil
}

func (p *PostgresBackend) MarkAsFailed(ctx context.Context, policyID uuid.UUID, reason string) error {
	err := p.queries.MarkAsFailed(ctx, sqlcgen.MarkAsFailedParams{
		PolicyID:      policyID,
		FailureReason: &reason,
	})
	if err != nil {
		return fmt.Errorf("failed to mark listing fee as failed: %w", err)
	}
	return nil
}

func (p *PostgresBackend) DeactivatePolicy(ctx context.Context, policyID uuid.UUID, reason string) error {
	err := p.queries.DeactivatePolicy(ctx, sqlcgen.DeactivatePolicyParams{
		ID:                 policyID,
		DeactivationReason: &reason,
	})
	if err != nil {
		return fmt.Errorf("failed to deactivate policy: %w", err)
	}
	return nil
}

func (p *PostgresBackend) GetPaidActivePolicyIDs(ctx context.Context) ([]uuid.UUID, error) {
	ids, err := p.queries.GetPaidActivePolicyIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query paid active policies: %w", err)
	}
	return ids, nil
}

func (p *PostgresBackend) HasActiveListingFee(ctx context.Context, publicKey, targetPluginID string) (bool, error) {
	exists, err := p.queries.HasActiveListingFee(ctx, sqlcgen.HasActiveListingFeeParams{
		PublicKey:      publicKey,
		TargetPluginID: targetPluginID,
	})
	if err != nil {
		return false, fmt.Errorf("failed to check active listing fee: %w", err)
	}
	return exists, nil
}

func (p *PostgresBackend) GetUnprocessedPolicyIDs(ctx context.Context) ([]uuid.UUID, error) {
	ids, err := p.queries.GetUnprocessedPolicyIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query unprocessed policies: %w", err)
	}
	return ids, nil
}

func (p *PostgresBackend) SyncSubmittedFees(ctx context.Context) (paid int64, failed int64, err error) {
	paid, err = p.queries.SyncPaidFees(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to sync paid fees: %w", err)
	}

	failed, err = p.queries.SyncFailedFees(ctx)
	if err != nil {
		return paid, 0, fmt.Errorf("failed to sync failed fees: %w", err)
	}

	return paid, failed, nil
}

func (p *PostgresBackend) UpdateConfirmations(ctx context.Context, policyID uuid.UUID, confirmations int) error {
	err := p.queries.UpdateConfirmations(ctx, sqlcgen.UpdateConfirmationsParams{
		PolicyID:      policyID,
		Confirmations: int32(confirmations),
	})
	if err != nil {
		return fmt.Errorf("failed to update confirmations: %w", err)
	}
	return nil
}

func toListingFee(row sqlcgen.ListingFee) *ListingFee {
	amount := new(big.Int)
	amount.SetString(row.Amount, 10)
	return &ListingFee{
		ID:             row.ID,
		PolicyID:       row.PolicyID,
		PublicKey:      row.PublicKey,
		TargetPluginID: row.TargetPluginID,
		Amount:         amount,
		Destination:    row.Destination,
		TxHash:         row.TxHash,
		BlockNumber:    row.BlockNumber,
		Confirmations:  int(row.Confirmations),
		Status:         row.Status,
		SubmittedAt:    row.SubmittedAt,
		PaidAt:         row.PaidAt,
		FailureReason:  row.FailureReason,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}

func toListingFees(rows []sqlcgen.ListingFee) []ListingFee {
	fees := make([]ListingFee, len(rows))
	for i, row := range rows {
		fees[i] = *toListingFee(row)
	}
	return fees
}
