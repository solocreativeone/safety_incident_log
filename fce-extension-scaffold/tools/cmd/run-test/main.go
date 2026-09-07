package main

import (
	"encoding/json"
	"flag"
	"strings"
	"time"

	"extension-scaffold/tools/pkg/configs"
	"extension-scaffold/tools/pkg/fccutils"
	"extension-scaffold/tools/pkg/support"
	instrutils "extension-scaffold/tools/pkg/utils"

	"github.com/ethereum/go-ethereum/common"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/pkg/errors"
)

type ClassifySeverityResponse struct {
	Severity       string `json:"severity"`
	DrivingFactor  string `json:"drivingFactor"`
	Summary        string `json:"summary"`
	Justification  string `json:"justification"`
	RecordHash     string `json:"recordHash"`
	ShouldLogChain bool   `json:"shouldLogChain"`
}

func main() {
	af := flag.String("a", configs.AddressesFile, "file with deployed addresses")
	cf := flag.String("c", "", "chain node url")
	rpcF := flag.String("rpc", "", "chain node json-rpc url")
	rpcUrlF := flag.String("rpc-url", "", "chain node json-rpc url")
	pf := flag.String("p", configs.ExtensionProxyURL, "extension proxy url")
	instructionSenderF := flag.String("instructionSender", "", "instructionSender address")
	flag.Parse()

	instructionSenderAddress := common.HexToAddress(*instructionSenderF)

	chainNodeURL := fccutils.GetRPCURL(*rpcF, *rpcUrlF, *cf)
	testSupport, err := support.DefaultSupport(*af, chainNodeURL)
	if err != nil {
		fccutils.FatalWithCause(err)
	}

	// --- Generic: configure contract -----------------------------------------
	logger.Infof("Setting extension ID on instruction sender...")
	err = instrutils.SetExtensionId(testSupport, instructionSenderAddress)
	if err != nil {
		if strings.Contains(err.Error(), "already set") || strings.Contains(err.Error(), "Extension ID already set") {
			logger.Infof("Extension ID already set on contract, continuing")
		} else {
			logger.Errorf("setExtensionId failed: %s", err)
			fccutils.FatalWithCause(errors.Errorf(
				"setExtensionId failed — is the extension registered? Check that pre-build.sh completed successfully. Error: %s", err))
		}
	}

	// --- Test case 1: LOW severity — should NOT be flagged for on-chain logging ---
	logger.Infof("Sending a LOW-severity incident report...")

	lowInstructionId, _, err := instrutils.SendClassifySeverity(
		testSupport, instructionSenderAddress,
		"Spilled some coffee near my desk, cleaned it up right away.",
	)
	if err != nil {
		fccutils.FatalWithCause(err)
	}
	logger.Infof("Instruction sent. ID: %s", lowInstructionId.Hex())

	time.Sleep(5 * time.Second)

	err = verifyClassifySeverityResult(*pf, lowInstructionId, false /* expectLogChain */)
	if err != nil {
		fccutils.FatalWithCause(err)
	}
	logger.Infof("Test passed: LOW-severity report classified, correctly not flagged for chain logging")

	// --- Test case 2: HIGH severity (near-miss) — SHOULD be flagged for on-chain logging ---
	logger.Infof("Sending a HIGH-severity incident report...")

	highInstructionId, _, err := instrutils.SendClassifySeverity(
		testSupport, instructionSenderAddress,
		"Forklift in bay 3 nearly clipped me, I jumped back just in time. Guard rail there has been missing for weeks.",
	)
	if err != nil {
		fccutils.FatalWithCause(err)
	}
	logger.Infof("Instruction sent. ID: %s", highInstructionId.Hex())

	time.Sleep(5 * time.Second)

	err = verifyClassifySeverityResult(*pf, highInstructionId, true /* expectLogChain */)
	if err != nil {
		fccutils.FatalWithCause(err)
	}
	logger.Infof("Test passed: HIGH-severity report classified, correctly flagged for chain logging")

	logger.Infof("All tests passed.")
}

// verifyClassifySeverityResult polls the proxy for the classification result
// and checks the response is well-formed. It does NOT assert an exact
// severity tier — that's an AI judgment call and can reasonably vary between
// runs even for a clearly-worded report — but it does assert the response
// shape is correct and that whether it was flagged for on-chain logging is
// directionally sane (severity != LOW implies shouldLogChain == true).
func verifyClassifySeverityResult(proxyURL string, instructionId common.Hash, expectLogChain bool) error {
	// --- Generic: poll proxy for result (do not modify) ---
	actionResponse, err := fccutils.ActionResult(proxyURL, instructionId)
	if err != nil {
		return err
	}
	actionResult := actionResponse.Result

	if actionResult.Status == 0 {
		return errors.Errorf("instruction processing failed: %s", actionResult.Log)
	}
	if actionResult.Status == 2 {
		return errors.New("instruction still pending after polling, expected completed")
	}

	if len(actionResult.Data) == 0 {
		return errors.New("expected response data but got none")
	}

	var resp ClassifySeverityResponse
	err = json.Unmarshal(actionResult.Data, &resp)
	if err != nil {
		return errors.Errorf("failed to unmarshal response: %s", err)
	}

	validSeverities := map[string]bool{"LOW": true, "MEDIUM": true, "HIGH": true, "CRITICAL": true}
	if !validSeverities[resp.Severity] {
		return errors.Errorf("expected severity to be one of LOW/MEDIUM/HIGH/CRITICAL, got %q", resp.Severity)
	}
	if resp.DrivingFactor == "" {
		return errors.New("expected non-empty DrivingFactor")
	}
	if resp.Summary == "" {
		return errors.New("expected non-empty Summary")
	}
	if resp.Justification == "" {
		return errors.New("expected non-empty Justification")
	}
	if !strings.HasPrefix(resp.RecordHash, "0x") || len(resp.RecordHash) != 66 {
		return errors.Errorf("expected a well-formed 0x-prefixed 32-byte hash, got %q", resp.RecordHash)
	}
	if resp.ShouldLogChain != expectLogChain {
		return errors.Errorf("expected ShouldLogChain=%v, got %v (severity was %s)", expectLogChain, resp.ShouldLogChain, resp.Severity)
	}

	logger.Infof("Response data: %+v", resp)

	return nil
}
