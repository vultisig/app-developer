# Developer Plugin

Plugin listing fee payment service for the Vultisig ecosystem.

## Capabilities
- One-time VULT token payment for plugin listing on the Vultisig marketplace
- Automatic payment detection via on-chain ERC-20 transfer indexing
- Payment status tracking (pending/paid)

## Supported Chains
- Ethereum (VULT ERC-20 token)

## Flow
1. Developer creates a policy (payment intent) via POST /plugin/policy
2. Developer queries GET /api/listing-fee/:id to get payment instructions
3. Developer sends exact VULT amount to treasury address from their vault
4. Worker detects payment on-chain and marks listing fee as paid
5. Payment status queryable via GET /api/listing-fee/by-scope
