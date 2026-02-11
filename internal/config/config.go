package config

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/spf13/viper"
	"github.com/vultisig/verifier/plugin/config"
	"github.com/vultisig/verifier/plugin/server"
	"github.com/vultisig/verifier/vault_config"
)

type ServerConfig struct {
	Server       server.Config             `mapstructure:"server" json:"server"`
	Database     config.Database           `mapstructure:"database" json:"database,omitempty"`
	Redis        config.Redis              `mapstructure:"redis" json:"redis,omitempty"`
	BlockStorage vault_config.BlockStorage `mapstructure:"block_storage" json:"block_storage,omitempty"`
	Verifier     config.Verifier           `mapstructure:"verifier" json:"verifier,omitempty"`
	Fee          FeeConfig                 `mapstructure:"fee" json:"fee,omitempty"`
}

type WorkerConfig struct {
	Database           config.Database           `mapstructure:"database" json:"database,omitempty"`
	Redis              config.Redis              `mapstructure:"redis" json:"redis,omitempty"`
	BlockStorage       vault_config.BlockStorage `mapstructure:"block_storage" json:"block_storage,omitempty"`
	VaultServiceConfig vault_config.Config       `mapstructure:"vault_service" json:"vault_service,omitempty"`
	Verifier           config.Verifier           `mapstructure:"verifier" json:"verifier,omitempty"`
	Fee                FeeConfig                 `mapstructure:"fee" json:"fee,omitempty"`
	TaskQueueName      string                    `mapstructure:"task_queue_name" json:"task_queue_name,omitempty"`
	ProcessingInterval time.Duration             `mapstructure:"processing_interval" json:"processing_interval,omitempty"`
	HealthPort         int                       `mapstructure:"health_port" json:"health_port,omitempty"`
}

type TxIndexerConfig struct {
	Database         config.Database `mapstructure:"database" json:"database,omitempty"`
	EthRpcURL        string          `mapstructure:"eth_rpc_url" json:"eth_rpc_url,omitempty"`
	Interval         time.Duration   `mapstructure:"interval" json:"interval,omitempty"`
	IterationTimeout time.Duration   `mapstructure:"iteration_timeout" json:"iteration_timeout,omitempty"`
	MarkLostAfter    time.Duration   `mapstructure:"mark_lost_after" json:"mark_lost_after,omitempty"`
	Concurrency      int             `mapstructure:"concurrency" json:"concurrency,omitempty"`
	HealthPort       int             `mapstructure:"health_port" json:"health_port,omitempty"`
}

type FeeConfig struct {
	VultTokenAddress string `mapstructure:"vult_token_address" json:"vult_token_address,omitempty"`
	TreasuryAddress  string `mapstructure:"treasury_address" json:"treasury_address,omitempty"`
	FeeAmount        string `mapstructure:"fee_amount" json:"fee_amount,omitempty"`
	EthRpcURL        string `mapstructure:"eth_rpc_url" json:"eth_rpc_url,omitempty"`
	ChainID          uint64 `mapstructure:"chain_id" json:"chain_id,omitempty"`
}

func ReadServerConfig() (*ServerConfig, error) {
	configName := os.Getenv("VS_CONFIG_NAME")
	if configName == "" {
		configName = "config"
	}

	addKeysToViper(viper.GetViper(), reflect.TypeOf(ServerConfig{}))
	viper.SetConfigName(configName)
	viper.AddConfigPath(".")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	viper.SetDefault("Server.VaultsFilePath", "vaults")
	setFeeDefaults()

	err := viper.ReadInConfig()
	if err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("fail to reading config file, %w", err)
		}
	}
	var cfg ServerConfig
	err = viper.Unmarshal(&cfg)
	if err != nil {
		return nil, fmt.Errorf("unable to decode into struct, %w", err)
	}
	return &cfg, nil
}

func ReadWorkerConfig() (*WorkerConfig, error) {
	configName := os.Getenv("VS_CONFIG_NAME")
	if configName == "" {
		configName = "config"
	}

	addKeysToViper(viper.GetViper(), reflect.TypeOf(WorkerConfig{}))
	viper.SetConfigName(configName)
	viper.AddConfigPath(".")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	viper.SetDefault("health_port", 8081)
	viper.SetDefault("processing_interval", "30s")
	setFeeDefaults()

	err := viper.ReadInConfig()
	if err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("fail to reading config file, %w", err)
		}
	}
	var cfg WorkerConfig
	err = viper.Unmarshal(&cfg)
	if err != nil {
		return nil, fmt.Errorf("unable to decode into struct, %w", err)
	}
	return &cfg, nil
}

func ReadTxIndexerConfig() (*TxIndexerConfig, error) {
	configName := os.Getenv("VS_CONFIG_NAME")
	if configName == "" {
		configName = "config"
	}

	addKeysToViper(viper.GetViper(), reflect.TypeOf(TxIndexerConfig{}))
	viper.SetConfigName(configName)
	viper.AddConfigPath(".")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	viper.SetDefault("interval", "15s")
	viper.SetDefault("iteration_timeout", "60s")
	viper.SetDefault("mark_lost_after", "30m")
	viper.SetDefault("concurrency", 5)
	viper.SetDefault("health_port", 8083)

	err := viper.ReadInConfig()
	if err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("fail to reading config file, %w", err)
		}
	}
	var cfg TxIndexerConfig
	err = viper.Unmarshal(&cfg)
	if err != nil {
		return nil, fmt.Errorf("unable to decode into struct, %w", err)
	}
	return &cfg, nil
}

func setFeeDefaults() {
	viper.SetDefault("fee.vult_token_address", "0xb788144DF611029C60b859DF47e79B7726C4DEBa")
	viper.SetDefault("fee.treasury_address", "")
	viper.SetDefault("fee.fee_amount", "")
	viper.SetDefault("fee.eth_rpc_url", "https://ethereum-rpc.publicnode.com")
	viper.SetDefault("fee.chain_id", 1)
}

func addKeysToViper(v *viper.Viper, t reflect.Type) {
	keys := getAllKeys(t)
	for _, key := range keys {
		v.SetDefault(key, "")
	}
}

func getAllKeys(t reflect.Type) []string {
	var result []string

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)

		tagName := f.Tag.Get("mapstructure")
		if tagName == "" || tagName == "-" {
			jsonTag := f.Tag.Get("json")
			if jsonTag != "" && jsonTag != "-" {
				tagName = strings.Split(jsonTag, ",")[0]
			}
		} else {
			tagName = strings.Split(tagName, ",")[0]
		}

		if tagName == "" || tagName == "-" {
			tagName = f.Name
		}

		n := strings.ToUpper(tagName)

		if reflect.Struct == f.Type.Kind() {
			subKeys := getAllKeys(f.Type)
			for _, k := range subKeys {
				result = append(result, n+"."+k)
			}
		} else {
			result = append(result, n)
		}
	}

	return result
}
