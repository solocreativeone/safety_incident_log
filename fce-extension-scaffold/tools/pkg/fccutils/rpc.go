package fccutils

import (
	"os"
	"strings"

	"extension-scaffold/tools/pkg/configs"

	"github.com/joho/godotenv"
)

// GetRPCURL resolves the Ethereum JSON-RPC provider endpoint using the following order of precedence:
// 1. Any non-empty flag value passed in flagValues (e.g. from -rpc, -rpc-url, or -c flags).
// 2. Environment variables, checked in order: RPC_URL, ETH_RPC_URL, CHAIN_URL.
// 3. Fallback default configs.ChainNodeURL ("http://127.0.0.1:8545").
func GetRPCURL(flagValues ...string) string {
	for _, v := range flagValues {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}

	// Ensure .env file is loaded if present
	_ = godotenv.Load()

	for _, envVar := range []string{"RPC_URL", "ETH_RPC_URL", "CHAIN_URL"} {
		if val := strings.TrimSpace(os.Getenv(envVar)); val != "" {
			return val
		}
	}

	return configs.ChainNodeURL
}
