package spec

import (
	"github.com/vultisig/vultisig-go/common"
)

const PluginDeveloper = "vultisig-developer-0000"

var SupportedChains = []common.Chain{common.Ethereum}

func getSupportedChainStrings() []string {
	var cc []string
	for _, c := range SupportedChains {
		cc = append(cc, c.String())
	}
	return cc
}

func toAnySlice(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}
