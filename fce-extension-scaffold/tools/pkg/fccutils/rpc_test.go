package fccutils

import (
	"testing"

	"extension-scaffold/tools/pkg/configs"
)

func TestGetRPCURL_FlagPrecedence(t *testing.T) {
	// Setup env vars to ensure flags override them
	t.Setenv("RPC_URL", "http://rpc-env:8545")
	t.Setenv("ETH_RPC_URL", "http://eth-rpc-env:8545")
	t.Setenv("CHAIN_URL", "http://chain-env:8545")

	// Test flag override
	got := GetRPCURL("", "http://custom-flag:8545", "")
	expected := "http://custom-flag:8545"
	if got != expected {
		t.Errorf("GetRPCURL() = %q, want %q", got, expected)
	}
}

func TestGetRPCURL_EnvPrecedence(t *testing.T) {
	tests := []struct {
		name     string
		envMap   map[string]string
		expected string
	}{
		{
			name: "RPC_URL takes priority over ETH_RPC_URL and CHAIN_URL",
			envMap: map[string]string{
				"RPC_URL":     "http://rpc-url:8545",
				"ETH_RPC_URL": "http://eth-rpc-url:8545",
				"CHAIN_URL":   "http://chain-url:8545",
			},
			expected: "http://rpc-url:8545",
		},
		{
			name: "ETH_RPC_URL takes priority over CHAIN_URL when RPC_URL is empty",
			envMap: map[string]string{
				"RPC_URL":     "",
				"ETH_RPC_URL": "http://eth-rpc-url:8545",
				"CHAIN_URL":   "http://chain-url:8545",
			},
			expected: "http://eth-rpc-url:8545",
		},
		{
			name: "CHAIN_URL used when RPC_URL and ETH_RPC_URL are empty",
			envMap: map[string]string{
				"RPC_URL":     "",
				"ETH_RPC_URL": "",
				"CHAIN_URL":   "http://chain-url:8545",
			},
			expected: "http://chain-url:8545",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("RPC_URL", tt.envMap["RPC_URL"])
			t.Setenv("ETH_RPC_URL", tt.envMap["ETH_RPC_URL"])
			t.Setenv("CHAIN_URL", tt.envMap["CHAIN_URL"])

			got := GetRPCURL("", "")
			if got != tt.expected {
				t.Errorf("GetRPCURL() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestGetRPCURL_Fallback(t *testing.T) {
	// Clear all RPC env vars
	t.Setenv("RPC_URL", "")
	t.Setenv("ETH_RPC_URL", "")
	t.Setenv("CHAIN_URL", "")

	got := GetRPCURL("", "")
	expected := configs.ChainNodeURL
	if got != expected {
		t.Errorf("GetRPCURL() = %q, want %q", got, expected)
	}
}

func TestGetRPCURL_Trimming(t *testing.T) {
	t.Setenv("RPC_URL", "")
	t.Setenv("ETH_RPC_URL", "")
	t.Setenv("CHAIN_URL", "  http://trimmed-url:8545  ")

	got := GetRPCURL("  ", "\t")
	expected := "http://trimmed-url:8545"
	if got != expected {
		t.Errorf("GetRPCURL() = %q, want %q", got, expected)
	}
}
