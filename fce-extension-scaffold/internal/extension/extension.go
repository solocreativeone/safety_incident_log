package extension

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"extension-scaffold/internal/config"
	"extension-scaffold/pkg/types"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/flare-foundation/go-flare-common/pkg/tee/instruction"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs"
	teetypes "github.com/flare-foundation/tee-node/pkg/types"
	teeutils "github.com/flare-foundation/tee-node/pkg/utils"

	"github.com/flare-foundation/tee-node/pkg/processorutils"
)

type Extension struct {
	mu     sync.RWMutex
	Server *http.Server

	classificationCount int
	lastSeverity         string
}

// classifyFunc is the classification implementation processClassifySeverity
// calls. Defaults to the real Gemini-backed implementation; tests override
// this package variable with a stub so unit tests never make live API calls.
var classifyFunc = classifyWithGemini

// --- DO NOT MODIFY: New(), actionHandler() are boilerplate.
func New(extensionPort, signPort int) *Extension {
	e := &Extension{}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /state", e.stateHandler)
	mux.HandleFunc("POST /action", e.actionHandler)

	e.Server = &http.Server{Addr: fmt.Sprintf(":%d", extensionPort), Handler: mux}
	return e
}

func (e *Extension) stateHandler(w http.ResponseWriter, r *http.Request) {
	e.mu.RLock()
	stateResponse := types.StateResponse{
		StateVersion: teeutils.ToHash(config.Version),
		State: types.State{
			ClassificationCount: e.classificationCount,
			LastSeverity:        e.lastSeverity,
		},
	}
	e.mu.RUnlock()

	err := json.NewEncoder(w).Encode(stateResponse)
	if err != nil {
		http.Error(w, fmt.Sprintf("sending response: %v", err), http.StatusInternalServerError)
		return
	}
}

func (e *Extension) processAction(action teetypes.Action) (int, []byte) {
	dataFixed, err := processorutils.Parse[instruction.DataFixed](action.Data.Message)
	if err != nil {
		return http.StatusBadRequest, []byte(fmt.Sprintf("decoding fixed data: %v", err))
	}

	switch {
	case dataFixed.OPType == teeutils.ToHash(config.OPTypeIncidentTriage):
		return e.processIncidentTriage(action, dataFixed)

	default:
		return http.StatusNotImplemented, []byte(fmt.Sprintf(
			"unsupported op type: received %s, expected %s (%s)",
			dataFixed.OPType.Hex(), teeutils.ToHash(config.OPTypeIncidentTriage).Hex(), config.OPTypeIncidentTriage,
		))
	}
}

// processIncidentTriage routes INCIDENT_TRIAGE instructions by OPCommand.
func (e *Extension) processIncidentTriage(action teetypes.Action, df *instruction.DataFixed) (int, []byte) {
	switch {
	case df.OPCommand == teeutils.ToHash(config.OPCommandClassifySeverity):
		ar := e.processClassifySeverity(action, df)
		b, _ := json.Marshal(ar)
		return http.StatusOK, b

	default:
		return http.StatusNotImplemented, []byte(fmt.Sprintf(
			"unsupported op command: received %s, expected %s (%s)",
			df.OPCommand.Hex(),
			teeutils.ToHash(config.OPCommandClassifySeverity).Hex(), config.OPCommandClassifySeverity,
		))
	}
}

// processClassifySeverity decodes the incident report, runs it through the
// AI severity rubric INSIDE the TEE (via classifyFunc — Gemini in
// production, a stub in tests), hashes the resulting record, and returns
// the classification. The raw report text never leaves this function —
// only the ClassifySeverityResponse (severity, factor, summary,
// justification, hash) is returned to the caller.
func (e *Extension) processClassifySeverity(action teetypes.Action, df *instruction.DataFixed) teetypes.ActionResult {
	var req types.ClassifySeverityRequest
	err := structs.DecodeTo(types.ClassifySeverityMessageArg, df.OriginalMessage, &req)
	if err != nil {
		return buildResult(action, df, nil, 0, fmt.Errorf("decoding request: %w", err))
	}

	if strings.TrimSpace(req.ReportText) == "" {
		return buildResult(action, df, nil, 0, fmt.Errorf("reportText must not be empty"))
	}

	classification, err := classifyFunc(req.ReportText)
	if err != nil {
		return buildResult(action, df, nil, 0, fmt.Errorf("classification failed: %w", err))
	}

	recordHash := hashRecord(classification)
	shouldLog := classification.Severity == "MEDIUM" || classification.Severity == "HIGH" || classification.Severity == "CRITICAL"

	resp := types.ClassifySeverityResponse{
		Severity:       classification.Severity,
		DrivingFactor:  classification.DrivingFactor,
		Summary:        classification.Summary,
		Justification:  classification.Justification,
		RecordHash:     recordHash,
		ShouldLogChain: shouldLog,
	}

	e.mu.Lock()
	e.classificationCount++
	e.lastSeverity = classification.Severity
	e.mu.Unlock()

	data, _ := json.Marshal(resp)
	return buildResult(action, df, data, 1, nil)
}

// --- Gemini classification (runs inside the TEE) ---

const rubricPrompt = `Classify incident report into LOW, MEDIUM, HIGH, or CRITICAL.
LOW: no injury/risk.
MEDIUM: minor injury possible, repairable damage.
HIGH: injury occurred/plausible, major damage, genuine near-miss.
CRITICAL: life-threatening, catastrophic failure risk, systemic gap.

Report:
"""%s"""

Respond with ONLY a JSON object:
{"severity": "LOW"|"MEDIUM"|"HIGH"|"CRITICAL", "driving_factor": "injury"|"equipment"|"near_miss", "summary": "...", "justification": "..."}`


type geminiClassification struct {
	Severity      string `json:"severity"`
	DrivingFactor string `json:"driving_factor"`
	Summary       string `json:"summary"`
	Justification string `json:"justification"`
}

type geminiRequest struct {
	Contents         []geminiContent  `json:"contents"`
	GenerationConfig generationConfig `json:"generationConfig,omitempty"`
}
type generationConfig struct {
	Temperature     float32 `json:"temperature"`
	MaxOutputTokens int     `json:"maxOutputTokens"`
}
type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}
type geminiPart struct {
	Text string `json:"text"`
}
type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []geminiPart `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

// classifyWithGemini calls the Gemini API from inside the TEE. The API key
// is injected as an environment variable on the extension-tee container —
// it never touches the public proxy or any external caller.
func classifyWithGemini(reportText string) (*geminiClassification, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY not set in TEE environment")
	}

	model := os.Getenv("GEMINI_MODEL")
	if model == "" {
		model = "gemini-flash-lite-latest"
	}

	prompt := fmt.Sprintf(rubricPrompt, reportText)
	reqBody := geminiRequest{
		Contents: []geminiContent{{Parts: []geminiPart{{Text: prompt}}}},
		GenerationConfig: generationConfig{
			Temperature:     0,
			MaxOutputTokens: 200,
		},
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	url := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		model, apiKey,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 1800*time.Millisecond)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 1800 * time.Millisecond}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		fmt.Printf("WARNING: Gemini API failed or timed out: %v. Using fallback.\n", err)
		return &geminiClassification{
			Severity:      "MEDIUM",
			DrivingFactor: "near_miss",
			Summary:       "Timeout or error occurred while contacting AI.",
			Justification: "Default fallback due to TEE processing timeout.",
		}, nil
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		fmt.Printf("WARNING: Gemini API returned %d. Using fallback.\n", httpResp.StatusCode)
		return &geminiClassification{
			Severity:      "MEDIUM",
			DrivingFactor: "near_miss",
			Summary:       fmt.Sprintf("API error %d.", httpResp.StatusCode),
			Justification: "Default fallback due to API error.",
		}, nil
	}

	var gr geminiResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&gr); err != nil {
		return nil, fmt.Errorf("decoding Gemini response: %w", err)
	}
	if len(gr.Candidates) == 0 || len(gr.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("empty Gemini response")
	}

	raw := strings.TrimSpace(gr.Candidates[0].Content.Parts[0].Text)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var classification geminiClassification
	if err := json.Unmarshal([]byte(raw), &classification); err != nil {
		return nil, fmt.Errorf("parsing classification JSON: %w (raw: %s)", err, raw)
	}

	return &classification, nil
}

// hashRecord computes a keccak256 hex hash of the canonical classification
// record — the same shape used by the offchain contract in chain.py, so
// TEE-produced and offchain-produced hashes stay comparable if you ever
// run both paths.
func hashRecord(c *geminiClassification) string {
	payload := fmt.Sprintf(
		`{"driving_factor":"%s","justification":"%s","severity":"%s","summary":"%s"}`,
		c.DrivingFactor, c.Justification, c.Severity, c.Summary,
	)
	hash := crypto.Keccak256([]byte(payload))
	return fmt.Sprintf("0x%x", hash)
}
