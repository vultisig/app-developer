package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	ecommon "github.com/ethereum/go-ethereum/common"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/sirupsen/logrus"
	"github.com/vultisig/app-developer/internal/config"
	"github.com/vultisig/app-developer/internal/db"
	"github.com/vultisig/app-developer/internal/evm"
	"github.com/vultisig/mobile-tss-lib/tss"
	evmsdk "github.com/vultisig/recipes/sdk/evm"
	"github.com/vultisig/verifier/plugin/policy"
	"github.com/vultisig/verifier/vault"
	"github.com/vultisig/vultisig-go/address"
	vcommon "github.com/vultisig/vultisig-go/common"
)

type Consumer struct {
	logger        *logrus.Logger
	policySvc     policy.Service
	signerService *evm.SignerService
	sdk           *evmsdk.SDK
	db            *db.PostgresBackend
	vaultStorage  vault.Storage
	vaultSecret   string
	feeConfig     config.FeeConfig
}

func NewConsumer(
	logger *logrus.Logger,
	policySvc policy.Service,
	signerService *evm.SignerService,
	sdk *evmsdk.SDK,
	database *db.PostgresBackend,
	vaultStorage vault.Storage,
	vaultSecret string,
	feeConfig config.FeeConfig,
) *Consumer {
	return &Consumer{
		logger:        logger.WithField("pkg", "worker.Consumer").Logger,
		policySvc:     policySvc,
		signerService: signerService,
		sdk:           sdk,
		db:            database,
		vaultStorage:  vaultStorage,
		vaultSecret:   vaultSecret,
		feeConfig:     feeConfig,
	}
}

type executePayload struct {
	PolicyID uuid.UUID `json:"policy_id"`
}

func (c *Consumer) HandleExecuteListingFee(_ context.Context, t *asynq.Task) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var payload executePayload
	err := json.Unmarshal(t.Payload(), &payload)
	if err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	c.logger.WithField("policy_id", payload.PolicyID).Info("executing listing fee payment")

	err = c.execute(ctx, payload.PolicyID)
	if err != nil {
		c.logger.WithError(err).WithField("policy_id", payload.PolicyID).Error("failed to execute listing fee")
		markErr := c.db.MarkAsFailed(ctx, payload.PolicyID, err.Error())
		if markErr != nil {
			c.logger.WithError(markErr).Error("failed to mark listing fee as failed")
		}
		return asynq.SkipRetry
	}

	return nil
}

func (c *Consumer) execute(ctx context.Context, policyID uuid.UUID) error {
	fee, err := c.db.GetListingFeeByPolicyID(ctx, policyID)
	if err != nil {
		return fmt.Errorf("failed to get listing fee: %w", err)
	}
	if fee == nil {
		return fmt.Errorf("listing fee not found for policy %s", policyID)
	}
	if fee.Status != "pending" {
		return fmt.Errorf("listing fee is not in pending state: %s", fee.Status)
	}

	pol, err := c.policySvc.GetPluginPolicy(ctx, policyID)
	if err != nil {
		return fmt.Errorf("failed to get policy: %w", err)
	}

	fromAddr, err := c.deriveAddress(pol.PublicKey, pol.PluginID.String())
	if err != nil {
		return fmt.Errorf("failed to derive sender address: %w", err)
	}

	toAddr := ecommon.HexToAddress(fee.Destination)
	tokenAddr := ecommon.HexToAddress(c.feeConfig.VultTokenAddress)

	unsignedTx, err := c.sdk.MakeTxTransferERC20(ctx, fromAddr, toAddr, tokenAddr, fee.Amount, 0)
	if err != nil {
		return fmt.Errorf("failed to build ERC-20 transfer: %w", err)
	}

	txHash, err := c.signerService.SignAndBroadcast(ctx, vcommon.Ethereum, *pol, unsignedTx)
	if err != nil {
		return fmt.Errorf("failed to sign and broadcast: %w", err)
	}

	err = c.db.MarkAsSubmitted(ctx, policyID, txHash)
	if err != nil {
		return fmt.Errorf("failed to mark as submitted: %w", err)
	}

	c.logger.WithFields(logrus.Fields{
		"policy_id": policyID,
		"tx_hash":   txHash,
	}).Info("listing fee payment submitted")

	return nil
}

func (c *Consumer) deriveAddress(publicKey string, pluginID string) (ecommon.Address, error) {
	vaultContent, err := c.vaultStorage.GetVault(vcommon.GetVaultBackupFilename(publicKey, pluginID))
	if err != nil {
		return ecommon.Address{}, fmt.Errorf("failed to get vault content: %w", err)
	}

	vlt, err := vcommon.DecryptVaultFromBackup(c.vaultSecret, vaultContent)
	if err != nil {
		return ecommon.Address{}, fmt.Errorf("failed to decrypt vault: %w", err)
	}

	childPub, err := tss.GetDerivedPubKey(publicKey, vlt.GetHexChainCode(), vcommon.Ethereum.GetDerivePath(), false)
	if err != nil {
		return ecommon.Address{}, fmt.Errorf("failed to get derived pubkey: %w", err)
	}

	addr, err := address.GetEVMAddress(childPub)
	if err != nil {
		return ecommon.Address{}, fmt.Errorf("failed to get address: %w", err)
	}

	return ecommon.HexToAddress(addr), nil
}
