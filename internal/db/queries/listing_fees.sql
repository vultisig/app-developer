-- name: CreateListingFee :exec
INSERT INTO listing_fees (policy_id, public_key, target_plugin_id, amount, destination, status)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (policy_id) DO NOTHING;

-- name: GetListingFeeByPolicyID :one
SELECT id, policy_id, public_key, target_plugin_id, amount, destination,
       tx_hash, block_number, confirmations, status,
       submitted_at, paid_at, failure_reason,
       created_at, updated_at
FROM listing_fees
WHERE policy_id = $1;

-- name: GetListingFeeByScope :one
SELECT id, policy_id, public_key, target_plugin_id, amount, destination,
       tx_hash, block_number, confirmations, status,
       submitted_at, paid_at, failure_reason,
       created_at, updated_at
FROM listing_fees
WHERE public_key = $1 AND target_plugin_id = $2
ORDER BY created_at DESC
LIMIT 1;

-- name: GetPendingListingFeeByScope :one
SELECT id, policy_id, public_key, target_plugin_id, amount, destination,
       tx_hash, block_number, confirmations, status,
       submitted_at, paid_at, failure_reason,
       created_at, updated_at
FROM listing_fees
WHERE public_key = $1 AND target_plugin_id = $2 AND status = 'pending'
LIMIT 1;

-- name: GetPendingListingFees :many
SELECT id, policy_id, public_key, target_plugin_id, amount, destination,
       tx_hash, block_number, confirmations, status,
       submitted_at, paid_at, failure_reason,
       created_at, updated_at
FROM listing_fees
WHERE status = 'pending';

-- name: GetSubmittedListingFees :many
SELECT id, policy_id, public_key, target_plugin_id, amount, destination,
       tx_hash, block_number, confirmations, status,
       submitted_at, paid_at, failure_reason,
       created_at, updated_at
FROM listing_fees
WHERE status = 'submitted';

-- name: MarkAsSubmitted :exec
UPDATE listing_fees
SET status = 'submitted', tx_hash = $2, submitted_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
WHERE policy_id = $1 AND status = 'pending';

-- name: MarkAsPaid :exec
UPDATE listing_fees
SET status = 'paid', block_number = $2, confirmations = $3, paid_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
WHERE policy_id = $1 AND status = 'submitted';

-- name: MarkAsFailed :exec
UPDATE listing_fees
SET status = 'failed', failure_reason = $2, updated_at = CURRENT_TIMESTAMP
WHERE policy_id = $1 AND status IN ('pending', 'submitted');

-- name: DeactivatePolicy :exec
UPDATE plugin_policies
SET active = false, deactivation_reason = $2
WHERE id = $1 AND active = true;

-- name: GetPaidActivePolicyIDs :many
SELECT lf.policy_id
FROM listing_fees lf
JOIN plugin_policies pp ON pp.id = lf.policy_id
WHERE lf.status = 'paid'
  AND pp.active = true;

-- name: HasActiveListingFee :one
SELECT EXISTS(
    SELECT 1 FROM listing_fees
    WHERE public_key = $1
      AND target_plugin_id = $2
      AND status IN ('pending', 'submitted', 'paid')
);

-- name: GetUnprocessedPolicyIDs :many
SELECT pp.id
FROM plugin_policies pp
LEFT JOIN listing_fees lf ON lf.policy_id = pp.id
WHERE pp.active = true
  AND lf.id IS NULL;

-- name: SyncPaidFees :execrows
UPDATE listing_fees lf
SET status = 'paid', paid_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
FROM tx_indexer ti
WHERE ti.policy_id = lf.policy_id
  AND lf.status = 'submitted'
  AND ti.status_onchain = 'SUCCESS';

-- name: SyncFailedFees :execrows
UPDATE listing_fees lf
SET status = 'failed',
    failure_reason = CASE WHEN ti.lost THEN 'transaction lost' ELSE 'transaction failed on-chain' END,
    updated_at = CURRENT_TIMESTAMP
FROM tx_indexer ti
WHERE ti.policy_id = lf.policy_id
  AND lf.status = 'submitted'
  AND (ti.status_onchain = 'FAIL' OR ti.lost = true);

-- name: UpdateConfirmations :exec
UPDATE listing_fees
SET confirmations = $2, updated_at = CURRENT_TIMESTAMP
WHERE policy_id = $1;
