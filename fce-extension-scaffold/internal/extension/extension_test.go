package extension

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"extension-scaffold/internal/config"
	"extension-scaffold/pkg/types"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	teetypes "github.com/flare-foundation/tee-node/pkg/types"
	teeutils "github.com/flare-foundation/tee-node/pkg/utils"
)

// toHash mirrors teeutils.ToHash for clarity: left-pads a string into a 32-byte hash.
func toHash(s string) common.Hash { return teeutils.ToHash(s) }

// buildTestAction constructs a teetypes.Action whose Data.Message is the
// JSON-encoded DataFixed payload. This is what processAction expects to parse.
func buildTestAction(opType, opCommand common.Hash, originalMessage []byte) teetypes.Action {
	type dataFixed struct {
		InstructionID      common.Hash    `json:"instructionId"`
		TeeID              common.Address `json:"teeId"`
		Timestamp          uint64         `json:"timestamp"`
		RewardEpochID      uint32         `json:"rewardEpochId"`
		OPType             common.Hash    `json:"opType"`
		OPCommand          common.Hash    `json:"opCommand"`
		Cosigners          []string       `json:"cosigners"`
		CosignersThreshold uint64         `json:"cosignersThreshold"`
		OriginalMessage    hexutil.Bytes  `json:"originalMessage"`
	}

	df := dataFixed{
		OPType:          opType,
		OPCommand:       opCommand,
		OriginalMessage: originalMessage,
	}
	msg, _ := json.Marshal(df)

	return teetypes.Action{
		Data: teetypes.ActionData{
			ID:            common.HexToHash("0x1234"),
			SubmissionTag: "submit",
			Message:       msg,
		},
	}
}

// abiEncodeClassifySeverity produces the ABI-encoded tuple (string reportText)
// matching the Solidity ClassifySeverityMessage struct.
func abiEncodeClassifySeverity(reportText string) []byte {
	args := abi.Arguments{types.ClassifySeverityMessageArg}
	type classifySeverity struct {
		ReportText string
	}
	encoded, _ := args.Pack(classifySeverity{ReportText: reportText})
	return encoded
}

// stubClassifier lets tests control classification output deterministically,
// without calling the real Gemini API. Call setStubClassifier in each test
// (or via t.Cleanup) so tests don't leak state into one another.
func setStubClassifier(t *testing.T, result *geminiClassification, err error) {
	t.Helper()
	original := classifyFunc
	classifyFunc = func(reportText string) (*geminiClassification, error) {
		return result, err
	}
	t.Cleanup(func() {
		classifyFunc = original
	})
}

// --- OPType/OPCommand Hash Debug Info ---

func TestProcessAction_UnknownOPType(t *testing.T) {
	e := &Extension{}
	action := buildTestAction(
		toHash("UNKNOWN_TYPE"),
		toHash(config.OPCommandClassifySeverity),
		nil,
	)

	status, body := e.processAction(action)

	if status != http.StatusNotImplemented {
		t.Fatalf("expected status %d, got %d", http.StatusNotImplemented, status)
	}

	bodyStr := string(body)
	t.Logf("501 body: %s", bodyStr)

	if !contains(bodyStr, "unsupported op type") {
		t.Error("expected body to contain 'unsupported op type'")
	}

	receivedHash := toHash("UNKNOWN_TYPE").Hex()
	if !contains(bodyStr, receivedHash) {
		t.Errorf("expected body to contain received hash %s", receivedHash)
	}

	expectedHash := toHash(config.OPTypeIncidentTriage).Hex()
	if !contains(bodyStr, expectedHash) {
		t.Errorf("expected body to contain expected hash %s", expectedHash)
	}

	if !contains(bodyStr, config.OPTypeIncidentTriage) {
		t.Errorf("expected body to contain %q", config.OPTypeIncidentTriage)
	}
}

func TestProcessAction_UnknownOPCommand(t *testing.T) {
	e := &Extension{}
	action := buildTestAction(
		toHash(config.OPTypeIncidentTriage),
		toHash("UNKNOWN_COMMAND"),
		nil,
	)

	status, body := e.processAction(action)

	if status != http.StatusNotImplemented {
		t.Fatalf("expected status %d, got %d", http.StatusNotImplemented, status)
	}

	bodyStr := string(body)
	t.Logf("501 body: %s", bodyStr)

	if !contains(bodyStr, "unsupported op command") {
		t.Error("expected body to contain 'unsupported op command'")
	}

	receivedHash := toHash("UNKNOWN_COMMAND").Hex()
	if !contains(bodyStr, receivedHash) {
		t.Errorf("expected body to contain received hash %s", receivedHash)
	}

	cmdHash := toHash(config.OPCommandClassifySeverity).Hex()
	if !contains(bodyStr, cmdHash) {
		t.Errorf("expected body to contain hash for %s: %s", config.OPCommandClassifySeverity, cmdHash)
	}
	if !contains(bodyStr, config.OPCommandClassifySeverity) {
		t.Errorf("expected body to contain command name %q", config.OPCommandClassifySeverity)
	}
}

// --- Valid Actions ---

func TestProcessAction_ValidClassifySeverity_Low(t *testing.T) {
	e := &Extension{}
	setStubClassifier(t, &geminiClassification{
		Severity:      "LOW",
		DrivingFactor: "equipment",
		Summary:       "Coffee spill, cleaned up immediately.",
		Justification: "No injury or risk, minor cosmetic spill.",
	}, nil)

	payload := abiEncodeClassifySeverity("Spilled some coffee near my desk, cleaned it up right away.")
	action := buildTestAction(
		toHash(config.OPTypeIncidentTriage),
		toHash(config.OPCommandClassifySeverity),
		payload,
	)

	status, body := e.processAction(action)
	if status != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, status, body)
	}

	var result teetypes.ActionResult
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("failed to unmarshal ActionResult: %v", err)
	}
	if result.Status != 1 {
		t.Fatalf("expected ActionResult.Status=1 (success), got %d: %s", result.Status, result.Log)
	}

	var resp types.ClassifySeverityResponse
	if err := json.Unmarshal(result.Data, &resp); err != nil {
		t.Fatalf("failed to unmarshal ClassifySeverityResponse: %v", err)
	}

	if resp.Severity != "LOW" {
		t.Errorf("expected severity LOW, got %q", resp.Severity)
	}
	if resp.ShouldLogChain {
		t.Error("expected ShouldLogChain=false for LOW severity")
	}
	if resp.RecordHash == "" || len(resp.RecordHash) != 66 { // "0x" + 64 hex chars
		t.Errorf("expected a well-formed 0x-prefixed 32-byte hash, got %q", resp.RecordHash)
	}
	t.Logf("Response: %+v", resp)
}

func TestProcessAction_ValidClassifySeverity_HighTriggersOnChain(t *testing.T) {
	e := &Extension{}
	setStubClassifier(t, &geminiClassification{
		Severity:      "HIGH",
		DrivingFactor: "near_miss",
		Summary:       "Forklift nearly struck an employee due to a missing guard rail.",
		Justification: "Serious injury narrowly avoided; systemic hazard unaddressed for weeks.",
	}, nil)

	payload := abiEncodeClassifySeverity("Forklift in bay 3 nearly clipped me, guard rail has been missing for weeks.")
	action := buildTestAction(
		toHash(config.OPTypeIncidentTriage),
		toHash(config.OPCommandClassifySeverity),
		payload,
	)

	status, body := e.processAction(action)
	if status != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, status, body)
	}

	var result teetypes.ActionResult
	json.Unmarshal(body, &result)
	if result.Status != 1 {
		t.Fatalf("expected ActionResult.Status=1 (success), got %d: %s", result.Status, result.Log)
	}

	var resp types.ClassifySeverityResponse
	json.Unmarshal(result.Data, &resp)

	if resp.Severity != "HIGH" {
		t.Errorf("expected severity HIGH, got %q", resp.Severity)
	}
	if resp.DrivingFactor != "near_miss" {
		t.Errorf("expected driving factor near_miss, got %q", resp.DrivingFactor)
	}
	if !resp.ShouldLogChain {
		t.Error("expected ShouldLogChain=true for HIGH severity")
	}
	t.Logf("Response: %+v", resp)
}

// --- Error Cases ---

func TestProcessClassifySeverity_EmptyReportText(t *testing.T) {
	e := &Extension{}
	// Classifier shouldn't even be called — empty text is rejected before that.
	setStubClassifier(t, nil, fmt.Errorf("classifier should not be called"))

	payload := abiEncodeClassifySeverity("")
	action := buildTestAction(
		toHash(config.OPTypeIncidentTriage),
		toHash(config.OPCommandClassifySeverity),
		payload,
	)

	status, body := e.processAction(action)
	if status != http.StatusOK {
		t.Fatalf("expected status %d (error is in ActionResult, not HTTP), got %d", http.StatusOK, status)
	}

	var result teetypes.ActionResult
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if result.Status != 0 {
		t.Fatalf("expected ActionResult.Status=0 (error), got %d", result.Status)
	}
	if !contains(result.Log, "reportText must not be empty") {
		t.Errorf("expected log to contain 'reportText must not be empty', got %q", result.Log)
	}
	t.Logf("Error log: %s", result.Log)
}

func TestProcessClassifySeverity_ClassifierError(t *testing.T) {
	e := &Extension{}
	setStubClassifier(t, nil, fmt.Errorf("Gemini returned status 503"))

	payload := abiEncodeClassifySeverity("Something happened near the loading dock.")
	action := buildTestAction(
		toHash(config.OPTypeIncidentTriage),
		toHash(config.OPCommandClassifySeverity),
		payload,
	)

	status, body := e.processAction(action)
	if status != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, status)
	}

	var result teetypes.ActionResult
	json.Unmarshal(body, &result)
	if result.Status != 0 {
		t.Fatalf("expected ActionResult.Status=0 (error), got %d", result.Status)
	}
	if !contains(result.Log, "classification failed") {
		t.Errorf("expected log to mention 'classification failed', got %q", result.Log)
	}
	t.Logf("Error log: %s", result.Log)
}

// --- State Tracking ---

func TestProcessAction_ClassificationCountIncrementsAcrossCalls(t *testing.T) {
	e := &Extension{}
	setStubClassifier(t, &geminiClassification{
		Severity:      "MEDIUM",
		DrivingFactor: "injury",
		Summary:       "Minor cut reported.",
		Justification: "Minor injury possible, avoided through normal caution.",
	}, nil)

	for i := 1; i <= 3; i++ {
		payload := abiEncodeClassifySeverity("Got a small cut, put a bandage on it.")
		action := buildTestAction(
			toHash(config.OPTypeIncidentTriage),
			toHash(config.OPCommandClassifySeverity),
			payload,
		)

		status, body := e.processAction(action)
		if status != http.StatusOK {
			t.Fatalf("call %d: expected status %d, got %d", i, http.StatusOK, status)
		}

		var result teetypes.ActionResult
		json.Unmarshal(body, &result)
		if result.Status != 1 {
			t.Fatalf("call %d: expected success, got status %d: %s", i, result.Status, result.Log)
		}
	}

	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.classificationCount != 3 {
		t.Errorf("expected classificationCount=3 after 3 calls, got %d", e.classificationCount)
	}
	if e.lastSeverity != "MEDIUM" {
		t.Errorf("expected lastSeverity=MEDIUM, got %q", e.lastSeverity)
	}
}

func TestProcessAction_InvalidDataMessage(t *testing.T) {
	e := &Extension{}

	action := teetypes.Action{
		Data: teetypes.ActionData{
			ID:      common.HexToHash("0xabcd"),
			Message: []byte(`not json at all`),
		},
	}

	status, body := e.processAction(action)

	if status != http.StatusBadRequest {
		t.Fatalf("expected status %d for invalid Data.Message, got %d: %s",
			http.StatusBadRequest, status, body)
	}

	bodyStr := string(body)
	if !contains(bodyStr, "decoding fixed data") {
		t.Errorf("expected body to mention 'decoding fixed data', got %q", bodyStr)
	}
	t.Logf("400 body: %s", bodyStr)
}

// contains is a simple helper to check substring presence.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
