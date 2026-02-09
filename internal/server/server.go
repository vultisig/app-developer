package server

import (
	"encoding/json"
	"math/big"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
	"github.com/vultisig/app-developer/internal/config"
	"github.com/vultisig/app-developer/internal/db"
	"github.com/vultisig/verifier/plugin/policy"
	"github.com/vultisig/verifier/plugin/tasks"
)

type ExecuteListingFeePayload struct {
	PolicyID uuid.UUID `json:"policy_id"`
}

type DeveloperAPI struct {
	policySvc policy.Service
	db        *db.PostgresBackend
	feeConfig config.FeeConfig
	asynq     *asynq.Client
	logger    *logrus.Logger
}

func NewDeveloperAPI(
	policySvc policy.Service,
	database *db.PostgresBackend,
	feeConfig config.FeeConfig,
	asynqClient *asynq.Client,
	logger *logrus.Logger,
) *DeveloperAPI {
	return &DeveloperAPI{
		policySvc: policySvc,
		db:        database,
		feeConfig: feeConfig,
		asynq:     asynqClient,
		logger:    logger,
	}
}

func (a *DeveloperAPI) RegisterRoutes(e *echo.Echo) {
	api := e.Group("/api")
	api.GET("/listing-fee/:id", a.handleGetListingFee)
	api.GET("/listing-fee/by-scope", a.handleGetListingFeeByScope)
	api.POST("/listing-fee/:id/execute", a.handleExecuteListingFee)
}

type listingFeeResponse struct {
	PolicyID       uuid.UUID           `json:"policy_id"`
	PublicKey      string              `json:"public_key"`
	TargetPluginID string              `json:"target_plugin_id"`
	Status         string              `json:"status"`
	Payment        paymentInstructions `json:"payment_instructions"`
	TxHash         *string             `json:"tx_hash,omitempty"`
	PaidAt         *time.Time          `json:"paid_at,omitempty"`
	FailureReason  *string             `json:"failure_reason,omitempty"`
}

type paymentInstructions struct {
	Destination string `json:"destination"`
	Amount      string `json:"amount"`
	VultToken   string `json:"vult_token"`
}

func (a *DeveloperAPI) handleGetListingFee(c echo.Context) error {
	idStr := c.Param("id")
	policyID, err := uuid.Parse(idStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid policy id"})
	}

	ctx := c.Request().Context()

	fee, err := a.db.GetListingFeeByPolicyID(ctx, policyID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "database error"})
	}

	if fee == nil {
		fee, err = a.lazyCreateListingFee(c, policyID)
		if err != nil {
			return err
		}
	}

	return c.JSON(http.StatusOK, toListingFeeResponse(fee, a.feeConfig))
}

func (a *DeveloperAPI) handleGetListingFeeByScope(c echo.Context) error {
	pubkey := c.QueryParam("pubkey")
	pluginID := c.QueryParam("pluginId")

	if pubkey == "" || pluginID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "pubkey and pluginId are required"})
	}

	fee, err := a.db.GetListingFeeByScope(c.Request().Context(), pubkey, pluginID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "database error"})
	}

	if fee == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "listing fee not found"})
	}

	return c.JSON(http.StatusOK, toListingFeeResponse(fee, a.feeConfig))
}

func (a *DeveloperAPI) handleExecuteListingFee(c echo.Context) error {
	idStr := c.Param("id")
	policyID, err := uuid.Parse(idStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid policy id"})
	}

	fee, err := a.db.GetListingFeeByPolicyID(c.Request().Context(), policyID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "database error"})
	}

	if fee == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "listing fee not found"})
	}

	if fee.Status != "pending" {
		return c.JSON(http.StatusConflict, map[string]string{
			"error":  "listing fee is not in pending state",
			"status": fee.Status,
		})
	}

	payload := ExecuteListingFeePayload{PolicyID: policyID}
	buf, err := json.Marshal(payload)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to marshal task"})
	}

	_, err = a.asynq.Enqueue(
		asynq.NewTask(tasks.TypePluginTransaction, buf),
		asynq.MaxRetry(0),
		asynq.Timeout(5*time.Minute),
		asynq.Retention(10*time.Minute),
		asynq.Queue(tasks.QUEUE_NAME),
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to enqueue task"})
	}

	return c.JSON(http.StatusAccepted, map[string]string{"status": "executing"})
}

func (a *DeveloperAPI) lazyCreateListingFee(c echo.Context, policyID uuid.UUID) (*db.ListingFee, error) {
	ctx := c.Request().Context()

	pol, err := a.policySvc.GetPluginPolicy(ctx, policyID)
	if err != nil {
		return nil, c.JSON(http.StatusNotFound, map[string]string{"error": "policy not found"})
	}

	recipe, err := pol.GetRecipe()
	if err != nil {
		return nil, c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid recipe"})
	}

	cfgMap := recipe.GetConfiguration().AsMap()
	targetPluginID, ok := cfgMap["target_plugin_id"].(string)
	if !ok || targetPluginID == "" {
		return nil, c.JSON(http.StatusBadRequest, map[string]string{"error": "missing target_plugin_id in configuration"})
	}

	existing, err := a.db.GetPendingListingFeeByScope(ctx, pol.PublicKey, targetPluginID)
	if err != nil {
		return nil, c.JSON(http.StatusInternalServerError, map[string]string{"error": "database error"})
	}
	if existing != nil {
		return existing, nil
	}

	amount := new(big.Int)
	amount.SetString(a.feeConfig.FeeAmount, 10)

	fee := db.ListingFee{
		PolicyID:       policyID,
		PublicKey:      pol.PublicKey,
		TargetPluginID: targetPluginID,
		Amount:         amount,
		Destination:    a.feeConfig.TreasuryAddress,
		Status:         "pending",
	}

	err = a.db.CreateListingFee(ctx, fee)
	if err != nil {
		return nil, c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create listing fee"})
	}

	created, err := a.db.GetListingFeeByPolicyID(ctx, policyID)
	if err != nil || created == nil {
		return nil, c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to read listing fee"})
	}

	return created, nil
}

func toListingFeeResponse(fee *db.ListingFee, feeConfig config.FeeConfig) listingFeeResponse {
	return listingFeeResponse{
		PolicyID:       fee.PolicyID,
		PublicKey:      fee.PublicKey,
		TargetPluginID: fee.TargetPluginID,
		Status:         fee.Status,
		Payment: paymentInstructions{
			Destination: fee.Destination,
			Amount:      fee.Amount.String(),
			VultToken:   feeConfig.VultTokenAddress,
		},
		TxHash:        fee.TxHash,
		PaidAt:        fee.PaidAt,
		FailureReason: fee.FailureReason,
	}
}
