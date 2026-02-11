package server

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
	"github.com/vultisig/app-developer/internal/config"
	"github.com/vultisig/app-developer/internal/db"
)

type DeveloperAPI struct {
	db        *db.PostgresBackend
	feeConfig config.FeeConfig
	logger    *logrus.Logger
}

func NewDeveloperAPI(
	database *db.PostgresBackend,
	feeConfig config.FeeConfig,
	logger *logrus.Logger,
) *DeveloperAPI {
	return &DeveloperAPI{
		db:        database,
		feeConfig: feeConfig,
		logger:    logger,
	}
}

func (a *DeveloperAPI) RegisterRoutes(e *echo.Echo) {
	api := e.Group("/api")
	api.GET("/listing-fee/by-scope", a.handleGetListingFeeByScope)
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
