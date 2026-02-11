package db

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
	query := `
		INSERT INTO listing_fees (policy_id, public_key, target_plugin_id, amount, destination, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (policy_id) DO NOTHING`

	_, err := p.pool.Exec(ctx, query,
		fee.PolicyID,
		fee.PublicKey,
		fee.TargetPluginID,
		fee.Amount.String(),
		fee.Destination,
		fee.Status,
	)
	if err != nil {
		return fmt.Errorf("failed to create listing fee: %w", err)
	}
	return nil
}

func (p *PostgresBackend) GetListingFeeByPolicyID(ctx context.Context, policyID uuid.UUID) (*ListingFee, error) {
	query := `
		SELECT id, policy_id, public_key, target_plugin_id, amount, destination,
		       tx_hash, block_number, confirmations, status,
		       submitted_at, paid_at, failure_reason,
		       created_at, updated_at
		FROM listing_fees
		WHERE policy_id = $1`

	row := p.pool.QueryRow(ctx, query, policyID)
	return scanListingFee(row)
}

func (p *PostgresBackend) GetListingFeeByScope(ctx context.Context, publicKey, pluginID string) (*ListingFee, error) {
	query := `
		SELECT id, policy_id, public_key, target_plugin_id, amount, destination,
		       tx_hash, block_number, confirmations, status,
		       submitted_at, paid_at, failure_reason,
		       created_at, updated_at
		FROM listing_fees
		WHERE public_key = $1 AND target_plugin_id = $2
		ORDER BY created_at DESC
		LIMIT 1`

	row := p.pool.QueryRow(ctx, query, publicKey, pluginID)
	return scanListingFee(row)
}

func (p *PostgresBackend) GetPendingListingFeeByScope(ctx context.Context, publicKey, pluginID string) (*ListingFee, error) {
	query := `
		SELECT id, policy_id, public_key, target_plugin_id, amount, destination,
		       tx_hash, block_number, confirmations, status,
		       submitted_at, paid_at, failure_reason,
		       created_at, updated_at
		FROM listing_fees
		WHERE public_key = $1 AND target_plugin_id = $2 AND status = 'pending'
		LIMIT 1`

	row := p.pool.QueryRow(ctx, query, publicKey, pluginID)
	return scanListingFee(row)
}

func (p *PostgresBackend) GetPendingListingFees(ctx context.Context) ([]ListingFee, error) {
	query := `
		SELECT id, policy_id, public_key, target_plugin_id, amount, destination,
		       tx_hash, block_number, confirmations, status,
		       submitted_at, paid_at, failure_reason,
		       created_at, updated_at
		FROM listing_fees
		WHERE status = 'pending'`

	rows, err := p.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending listing fees: %w", err)
	}
	defer rows.Close()

	return scanListingFees(rows)
}

func (p *PostgresBackend) GetSubmittedListingFees(ctx context.Context) ([]ListingFee, error) {
	query := `
		SELECT id, policy_id, public_key, target_plugin_id, amount, destination,
		       tx_hash, block_number, confirmations, status,
		       submitted_at, paid_at, failure_reason,
		       created_at, updated_at
		FROM listing_fees
		WHERE status = 'submitted'`

	rows, err := p.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query submitted listing fees: %w", err)
	}
	defer rows.Close()

	return scanListingFees(rows)
}

func (p *PostgresBackend) MarkAsSubmitted(ctx context.Context, policyID uuid.UUID, txHash string) error {
	query := `
		UPDATE listing_fees
		SET status = 'submitted', tx_hash = $2, submitted_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE policy_id = $1 AND status = 'pending'`

	_, err := p.pool.Exec(ctx, query, policyID, txHash)
	if err != nil {
		return fmt.Errorf("failed to mark listing fee as submitted: %w", err)
	}
	return nil
}

func (p *PostgresBackend) MarkAsPaid(ctx context.Context, policyID uuid.UUID, blockNum int64, confirmations int) error {
	query := `
		UPDATE listing_fees
		SET status = 'paid', block_number = $2, confirmations = $3, paid_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE policy_id = $1 AND status = 'submitted'`

	_, err := p.pool.Exec(ctx, query, policyID, blockNum, confirmations)
	if err != nil {
		return fmt.Errorf("failed to mark listing fee as paid: %w", err)
	}
	return nil
}

func (p *PostgresBackend) MarkAsFailed(ctx context.Context, policyID uuid.UUID, reason string) error {
	query := `
		UPDATE listing_fees
		SET status = 'failed', failure_reason = $2, updated_at = CURRENT_TIMESTAMP
		WHERE policy_id = $1 AND status IN ('pending', 'submitted')`

	_, err := p.pool.Exec(ctx, query, policyID, reason)
	if err != nil {
		return fmt.Errorf("failed to mark listing fee as failed: %w", err)
	}
	return nil
}

func (p *PostgresBackend) DeactivatePolicy(ctx context.Context, policyID uuid.UUID, reason string) error {
	query := `
		UPDATE plugin_policies
		SET active = false, deactivation_reason = $2
		WHERE id = $1 AND active = true`

	_, err := p.pool.Exec(ctx, query, policyID, reason)
	if err != nil {
		return fmt.Errorf("failed to deactivate policy: %w", err)
	}
	return nil
}

func (p *PostgresBackend) GetPaidActivePolicyIDs(ctx context.Context) ([]uuid.UUID, error) {
	query := `
		SELECT lf.policy_id
		FROM listing_fees lf
		JOIN plugin_policies pp ON pp.id = lf.policy_id
		WHERE lf.status = 'paid'
		  AND pp.active = true`

	rows, err := p.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query paid active policies: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		scanErr := rows.Scan(&id)
		if scanErr != nil {
			return nil, fmt.Errorf("failed to scan policy id: %w", scanErr)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (p *PostgresBackend) HasActiveListingFee(ctx context.Context, publicKey, targetPluginID string) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM listing_fees
			WHERE public_key = $1
			  AND target_plugin_id = $2
			  AND status IN ('pending', 'submitted', 'paid')
		)`

	var exists bool
	err := p.pool.QueryRow(ctx, query, publicKey, targetPluginID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check active listing fee: %w", err)
	}
	return exists, nil
}

func (p *PostgresBackend) GetUnprocessedPolicyIDs(ctx context.Context) ([]uuid.UUID, error) {
	query := `
		SELECT pp.id
		FROM plugin_policies pp
		LEFT JOIN listing_fees lf ON lf.policy_id = pp.id
		WHERE pp.active = true
		  AND lf.id IS NULL`

	rows, err := p.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query unprocessed policies: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		scanErr := rows.Scan(&id)
		if scanErr != nil {
			return nil, fmt.Errorf("failed to scan policy id: %w", scanErr)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (p *PostgresBackend) SyncSubmittedFees(ctx context.Context) (paid int64, failed int64, err error) {
	paidQuery := `
		UPDATE listing_fees lf
		SET status = 'paid', paid_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		FROM tx_indexer ti
		WHERE ti.policy_id = lf.policy_id
		  AND lf.status = 'submitted'
		  AND ti.status_onchain = 'SUCCESS'`

	paidResult, err := p.pool.Exec(ctx, paidQuery)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to sync paid fees: %w", err)
	}

	failedQuery := `
		UPDATE listing_fees lf
		SET status = 'failed',
		    failure_reason = CASE WHEN ti.lost THEN 'transaction lost' ELSE 'transaction failed on-chain' END,
		    updated_at = CURRENT_TIMESTAMP
		FROM tx_indexer ti
		WHERE ti.policy_id = lf.policy_id
		  AND lf.status = 'submitted'
		  AND (ti.status_onchain = 'FAIL' OR ti.lost = true)`

	failedResult, err := p.pool.Exec(ctx, failedQuery)
	if err != nil {
		return paidResult.RowsAffected(), 0, fmt.Errorf("failed to sync failed fees: %w", err)
	}

	return paidResult.RowsAffected(), failedResult.RowsAffected(), nil
}

func (p *PostgresBackend) UpdateConfirmations(ctx context.Context, policyID uuid.UUID, confirmations int) error {
	query := `
		UPDATE listing_fees
		SET confirmations = $2, updated_at = CURRENT_TIMESTAMP
		WHERE policy_id = $1`

	_, err := p.pool.Exec(ctx, query, policyID, confirmations)
	if err != nil {
		return fmt.Errorf("failed to update confirmations: %w", err)
	}
	return nil
}

func scanListingFee(row pgx.Row) (*ListingFee, error) {
	var f ListingFee
	var amountStr string
	err := row.Scan(
		&f.ID, &f.PolicyID, &f.PublicKey, &f.TargetPluginID,
		&amountStr, &f.Destination,
		&f.TxHash, &f.BlockNumber, &f.Confirmations,
		&f.Status,
		&f.SubmittedAt, &f.PaidAt, &f.FailureReason,
		&f.CreatedAt, &f.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan listing fee: %w", err)
	}
	f.Amount = new(big.Int)
	f.Amount.SetString(amountStr, 10)
	return &f, nil
}

func scanListingFees(rows pgx.Rows) ([]ListingFee, error) {
	var fees []ListingFee
	for rows.Next() {
		var f ListingFee
		var amountStr string
		err := rows.Scan(
			&f.ID, &f.PolicyID, &f.PublicKey, &f.TargetPluginID,
			&amountStr, &f.Destination,
			&f.TxHash, &f.BlockNumber, &f.Confirmations,
			&f.Status,
			&f.SubmittedAt, &f.PaidAt, &f.FailureReason,
			&f.CreatedAt, &f.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan listing fee: %w", err)
		}
		f.Amount = new(big.Int)
		f.Amount.SetString(amountStr, 10)
		fees = append(fees, f)
	}
	return fees, nil
}
