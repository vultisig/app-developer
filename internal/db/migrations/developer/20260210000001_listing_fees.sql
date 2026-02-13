-- +goose Up
-- +goose StatementBegin
CREATE TABLE listing_fees (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    policy_id UUID NOT NULL UNIQUE,
    public_key TEXT NOT NULL,
    target_plugin_id TEXT NOT NULL,
    amount NUMERIC(78,0) NOT NULL,
    destination TEXT NOT NULL,
    tx_hash TEXT UNIQUE,
    block_number BIGINT,
    confirmations INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'pending',
    submitted_at TIMESTAMP,
    paid_at TIMESTAMP,
    failure_reason TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_listing_fees_status ON listing_fees(status);
CREATE INDEX idx_listing_fees_public_key ON listing_fees(public_key);
CREATE UNIQUE INDEX idx_listing_fees_scope_pending
    ON listing_fees(public_key, target_plugin_id) WHERE status = 'pending';
-- +goose StatementEnd

-- +goose Down
