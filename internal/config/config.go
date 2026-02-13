package config

type FeeConfig struct {
	VultTokenAddress string `envconfig:"VULT_TOKEN_ADDRESS" default:"0xb788144DF611029C60b859DF47e79B7726C4DEBa"`
	TreasuryAddress  string `envconfig:"TREASURY_ADDRESS"`
	Amount           string
	EthRpcURL        string `envconfig:"ETH_RPC_URL" default:"https://ethereum-rpc.publicnode.com"`
	ChainID          uint64 `envconfig:"CHAIN_ID" default:"1"`
}
