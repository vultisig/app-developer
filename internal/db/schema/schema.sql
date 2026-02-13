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

CREATE TABLE plugin_policies (
    id UUID PRIMARY KEY,
    active BOOLEAN NOT NULL DEFAULT true,
    deactivation_reason TEXT
);

CREATE TABLE tx_indexer (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    policy_id UUID NOT NULL,
    status_onchain TEXT NOT NULL DEFAULT '',
    lost BOOLEAN NOT NULL DEFAULT false
);
