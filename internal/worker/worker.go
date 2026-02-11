package worker

import (
	"context"
	"fmt"
	"math/big"
	"time"

	ecommon "github.com/ethereum/go-ethereum/common"
	"github.com/google/uuid"
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

func (c *Consumer) Run(ctx context.Context, interval time.Duration) {
	if interval == 0 {
		interval = 30 * time.Second
	}
	c.logger.WithField("interval", interval).Info("listing fee processor started")
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.process(ctx)
		case <-ctx.Done():
			c.logger.Info("listing fee processor stopped")
			return
		}
	}
}

func (c *Consumer) process(ctx context.Context) {
	c.createListingFeesForNewPolicies(ctx)
	c.executePendingFees(ctx)
	c.syncSubmittedFees(ctx)
	c.deactivatePaidPolicies(ctx)
}

func (c *Consumer) createListingFeesForNewPolicies(ctx context.Context) {
	policyIDs, err := c.db.GetUnprocessedPolicyIDs(ctx)
	if err != nil {
		c.logger.WithError(err).Error("failed to get unprocessed policies")
		return
	}

	for _, policyID := range policyIDs {
		err = c.createListingFee(ctx, policyID)
		if err != nil {
			c.logger.WithError(err).WithField("policy_id", policyID).Error("failed to create listing fee")
		}
	}
}

func (c *Consumer) createListingFee(ctx context.Context, policyID uuid.UUID) error {
	pol, err := c.policySvc.GetPluginPolicy(ctx, policyID)
	if err != nil {
		return fmt.Errorf("failed to get policy: %w", err)
	}

	recipe, err := pol.GetRecipe()
	if err != nil {
		return fmt.Errorf("failed to get recipe: %w", err)
	}

	cfgMap := recipe.GetConfiguration().AsMap()
	targetPluginID, ok := cfgMap["targetPluginId"].(string)
	if !ok || targetPluginID == "" {
		return fmt.Errorf("missing targetPluginId in configuration")
	}

	amount := new(big.Int)
	amount.SetString(c.feeConfig.FeeAmount, 10)

	fee := db.ListingFee{
		PolicyID:       policyID,
		PublicKey:      pol.PublicKey,
		TargetPluginID: targetPluginID,
		Amount:         amount,
		Destination:    c.feeConfig.TreasuryAddress,
		Status:         "pending",
	}

	err = c.db.CreateListingFee(ctx, fee)
	if err != nil {
		return fmt.Errorf("failed to create listing fee: %w", err)
	}

	c.logger.WithFields(logrus.Fields{
		"policy_id":        policyID,
		"target_plugin_id": targetPluginID,
	}).Info("listing fee created")

	return nil
}

func (c *Consumer) syncSubmittedFees(ctx context.Context) {
	paid, failed, err := c.db.SyncSubmittedFees(ctx)
	if err != nil {
		c.logger.WithError(err).Error("failed to sync submitted fees")
		return
	}
	if paid > 0 || failed > 0 {
		c.logger.WithFields(logrus.Fields{
			"paid":   paid,
			"failed": failed,
		}).Info("synced submitted fees from tx_indexer")
	}
}

// deactivatePaidPolicies marks policies as inactive once their listing fee is paid.
// This also prevents charging a user twice: if a duplicate policy is created for the
// same plugin, the paid policy is deactivated before the duplicate can be executed.
func (c *Consumer) deactivatePaidPolicies(ctx context.Context) {
	policyIDs, err := c.db.GetPaidActivePolicyIDs(ctx)
	if err != nil {
		c.logger.WithError(err).Error("failed to get paid active policies")
		return
	}

	for _, policyID := range policyIDs {
		err = c.db.DeactivatePolicy(ctx, policyID, "completed")
		if err != nil {
			c.logger.WithError(err).WithField("policy_id", policyID).Error("failed to deactivate policy")
			continue
		}
		c.logger.WithField("policy_id", policyID).Info("policy deactivated (listing fee paid)")
	}
}

func (c *Consumer) executePendingFees(ctx context.Context) {
	fees, err := c.db.GetPendingListingFees(ctx)
	if err != nil {
		c.logger.WithError(err).Error("failed to get pending listing fees")
		return
	}

	for _, fee := range fees {
		executeErr := c.execute(ctx, fee.PolicyID)
		if executeErr != nil {
			c.logger.WithError(executeErr).WithField("policy_id", fee.PolicyID).Error("failed to execute listing fee")
			markErr := c.db.MarkAsFailed(ctx, fee.PolicyID, executeErr.Error())
			if markErr != nil {
				c.logger.WithError(markErr).Error("failed to mark listing fee as failed")
			}
		}
	}
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
