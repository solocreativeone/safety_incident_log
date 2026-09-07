// Package types contains types that could be useful to other apps when interacting with this extension.
package types

import (
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

// ClassifySeverityRequest is the ABI-decoded payload sent via the Solidity
// contract (mirrors ClassifySeverityMessage in InstructionSender.sol).
type ClassifySeverityRequest struct {
	ReportText string `json:"reportText"`
}

// ClassifySeverityResponse is the JSON payload returned in ActionResult.Data.
// Note: the raw report text is deliberately NOT included here — only the
// classification output leaves the TEE. This is what keeps the worker's
// report confidential end-to-end.
type ClassifySeverityResponse struct {
	Severity       string `json:"severity"`       // LOW | MEDIUM | HIGH | CRITICAL
	DrivingFactor  string `json:"drivingFactor"`   // injury | equipment | near_miss
	Summary        string `json:"summary"`         // one-sentence plain-language summary
	Justification  string `json:"justification"`   // one-sentence reasoning for the tier
	RecordHash     string `json:"recordHash"`      // keccak256 hex of the canonical record, for onchain logging
	ShouldLogChain bool   `json:"shouldLogChain"`  // true if severity >= MEDIUM
}

// ClassifySeverityMessageArg describes the ABI layout of
// ClassifySeverityMessage from the Solidity contract.
var ClassifySeverityMessageArg abi.Argument

func init() {
	tupleTy, _ := abi.NewType("tuple", "", []abi.ArgumentMarshaling{
		{Name: "reportText", Type: "string"},
	})
	ClassifySeverityMessageArg = abi.Argument{Type: tupleTy}
}

// State holds the extension's observable state, returned by GET /state.
type State struct {
	ClassificationCount int    `json:"classificationCount"`
	LastSeverity        string `json:"lastSeverity"`
}

// --- DO NOT MODIFY below this line. ---

// StateResponse is the envelope returned by GET /state.
type StateResponse struct {
	StateVersion common.Hash `json:"stateVersion"`
	State        State       `json:"state"`
}
