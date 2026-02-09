package spec

import (
	"context"
	"fmt"
	"strings"

	rtypes "github.com/vultisig/recipes/types"
	"github.com/vultisig/verifier/plugin"
	"github.com/vultisig/verifier/plugin/tx_indexer/pkg/conv"
	"github.com/vultisig/verifier/types"
)

type Spec struct {
	plugin.Unimplemented
}

func NewSpec() *Spec {
	return &Spec{}
}

func (s *Spec) GetPluginID() string {
	return PluginDeveloper
}

func (s *Spec) GetSkills() string {
	return skillsMD
}

func (s *Spec) GetRecipeSpecification() (*rtypes.RecipeSchema, error) {
	cfg, err := plugin.RecipeConfiguration(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"target_plugin_id": map[string]any{
				"type":        "string",
				"description": "The plugin ID to pay listing fee for",
			},
		},
		"required": []any{"target_plugin_id"},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build recipe config: %w", err)
	}

	return &rtypes.RecipeSchema{
		Version:            1,
		PluginId:           PluginDeveloper,
		PluginName:         "Developer Listing Fee",
		PluginVersion:      1,
		SupportedResources: s.buildSupportedResources(),
		Configuration:      cfg,
		Requirements: &rtypes.PluginRequirements{
			MinVultisigVersion: 1,
			SupportedChains:    getSupportedChainStrings(),
		},
		Permissions: []*rtypes.Permission{
			{
				Id:          "transaction_signing",
				Label:       "Access to transaction signing",
				Description: "The app can initiate transactions to send assets in your Vault",
			},
			{
				Id:          "balance_visibility",
				Label:       "Vault balance visibility",
				Description: "The app can view Vault balances",
			},
		},
	}, nil
}

func (s *Spec) ValidatePluginPolicy(pol types.PluginPolicy) error {
	spec, err := s.GetRecipeSpecification()
	if err != nil {
		return fmt.Errorf("failed to get recipe spec: %w", err)
	}
	return plugin.ValidatePluginPolicy(pol, spec)
}

// Suggest returns policy constraints for a one-time listing fee payment.
// The asset and amount constraints use FIXED type but values are left empty here
// because the actual VULT token address and fee amount are server-side config.
// The payment indexer enforces exact matching against those config values.
func (s *Spec) Suggest(_ context.Context, cfg map[string]any) (*rtypes.PolicySuggest, error) {
	_, ok := cfg["target_plugin_id"].(string)
	if !ok {
		return nil, fmt.Errorf("'target_plugin_id' is required")
	}

	fromAddress, _ := cfg["from_address"].(string)

	chainLowercase := strings.ToLower(SupportedChains[0].String())

	constraints := []*rtypes.ParameterConstraint{
		{
			ParameterName: "asset",
			Constraint: &rtypes.Constraint{
				Type:     rtypes.ConstraintType_CONSTRAINT_TYPE_FIXED,
				Required: true,
			},
		},
		{
			ParameterName: "from_address",
			Constraint: &rtypes.Constraint{
				Type: rtypes.ConstraintType_CONSTRAINT_TYPE_FIXED,
				Value: &rtypes.Constraint_FixedValue{
					FixedValue: fromAddress,
				},
				Required: true,
			},
		},
		{
			ParameterName: "amount",
			Constraint: &rtypes.Constraint{
				Type:     rtypes.ConstraintType_CONSTRAINT_TYPE_FIXED,
				Required: true,
			},
		},
		{
			ParameterName: "to_address",
			Constraint: &rtypes.Constraint{
				Type:     rtypes.ConstraintType_CONSTRAINT_TYPE_MAGIC_CONSTANT,
				Required: true,
			},
		},
	}

	rule := &rtypes.Rule{
		Resource:             chainLowercase + ".send",
		Effect:               rtypes.Effect_EFFECT_ALLOW,
		ParameterConstraints: constraints,
		Target: &rtypes.Target{
			TargetType: rtypes.TargetType_TARGET_TYPE_UNSPECIFIED,
		},
	}

	return &rtypes.PolicySuggest{
		RateLimitWindow: conv.Ptr(uint32(90)),
		MaxTxsPerWindow: conv.Ptr(uint32(1)),
		Rules:           []*rtypes.Rule{rule},
	}, nil
}

func (s *Spec) buildSupportedResources() []*rtypes.ResourcePattern {
	var resources []*rtypes.ResourcePattern
	for _, chain := range SupportedChains {
		chainNameLower := strings.ToLower(chain.String())

		resources = append(resources, &rtypes.ResourcePattern{
			ResourcePath: &rtypes.ResourcePath{
				ChainId:    chainNameLower,
				ProtocolId: "send",
				FunctionId: "Access to transaction signing",
				Full:       chainNameLower + ".send",
			},
			Target: rtypes.TargetType_TARGET_TYPE_UNSPECIFIED,
			ParameterCapabilities: []*rtypes.ParameterConstraintCapability{
				{
					ParameterName:  "asset",
					SupportedTypes: rtypes.ConstraintType_CONSTRAINT_TYPE_FIXED,
					Required:       true,
				},
				{
					ParameterName:  "from_address",
					SupportedTypes: rtypes.ConstraintType_CONSTRAINT_TYPE_FIXED,
					Required:       true,
				},
				{
					ParameterName:  "amount",
					SupportedTypes: rtypes.ConstraintType_CONSTRAINT_TYPE_FIXED,
					Required:       true,
				},
				{
					ParameterName:  "to_address",
					SupportedTypes: rtypes.ConstraintType_CONSTRAINT_TYPE_MAGIC_CONSTANT,
					Required:       true,
				},
			},
			Required: true,
		})
	}

	return resources
}
