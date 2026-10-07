// Command fake-decision-provider is a small, out-of-module external decision
// provider used ONLY to prove the provider-neutral integration seam end to end.
//
// It lives in its own Go module (testdata/fake-decision-provider) and imports
// nothing from agentic-sop: it satisfies only the documented JSON protocol — a
// request DTO on stdin, a result DTO on stdout — so it demonstrates that an
// external module needs no SOP Go type and can never reach SOP policy, lifecycle,
// approval, or state. It mutates nothing except its own stdout/stderr and the
// optional diagnostic files named on the command line.
//
// Usage: fake-decision-provider [mode] [requestCapturePath] [pidPath]
//
// Modes (all deterministic; no network, no model, no credentials):
//
//	decide (default)      adapter-like: classify the request subject and signals
//	low|medium|high|human a fixed valid choice at confidence 0.9
//	conf-zero|conf-one    a valid choice at confidence 0 or 1
//	unsupported           status UNSUPPORTED
//	indeterminate         status INDETERMINATE
//	unknown-choice        status OK with an unknown choice
//	bad-confidence        status OK with an out-of-range confidence
//	malformed             non-JSON stdout
//	empty                 no stdout
//	fail                  a non-zero exit with a diagnostic
//	panic                 a crash with a diagnostic
//	hostile               a valid LOW with command-like diagnostics
//	sleep                 write the pid, then sleep (for timeout/cancellation)
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// contractVersion is the SEAM-002 contract version this fixture implements. The
// fixture declares it as its own constant: it shares no Go type with SOP.
const contractVersion = 1

// request is the fixture's own view of the documented request DTO. It is a plain
// struct, not a SOP type.
type request struct {
	ContractVersion int                `json:"contract_version"`
	Kind            string             `json:"kind"`
	Question        string             `json:"question"`
	Signals         map[string]float64 `json:"signals"`
	Choices         []string           `json:"choices"`
}

// result is the fixture's own view of the documented result DTO.
type result struct {
	ContractVersion int               `json:"contract_version"`
	Status          string            `json:"status"`
	Kind            string            `json:"kind,omitempty"`
	Choice          string            `json:"choice,omitempty"`
	Confidence      *float64          `json:"confidence,omitempty"`
	Diagnostics     map[string]string `json:"diagnostics,omitempty"`
	Error           string            `json:"error,omitempty"`
}

func main() {
	mode := "decide"
	if len(os.Args) > 1 && strings.TrimSpace(os.Args[1]) != "" {
		mode = strings.TrimSpace(os.Args[1])
	}
	capture := ""
	if len(os.Args) > 2 {
		capture = os.Args[2]
	}
	pidPath := ""
	if len(os.Args) > 3 {
		pidPath = os.Args[3]
	}

	raw, _ := io.ReadAll(os.Stdin)
	if capture != "" {
		_ = os.WriteFile(capture, raw, 0o644)
	}
	var req request
	_ = json.Unmarshal(raw, &req)

	switch mode {
	case "sleep":
		if pidPath != "" {
			_ = os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0o644)
		}
		time.Sleep(60 * time.Second)
	case "fail":
		fmt.Fprintln(os.Stderr, "fake provider is down")
		os.Exit(3)
	case "panic":
		panic("fake provider crashed")
	case "malformed":
		fmt.Print("this is not json")
	case "empty":
		// No stdout at all.
	case "unsupported":
		emit(result{ContractVersion: contractVersion, Status: "UNSUPPORTED", Kind: req.Kind, Error: "kind not supported"})
	case "indeterminate":
		emit(result{ContractVersion: contractVersion, Status: "INDETERMINATE", Kind: req.Kind})
	case "unknown-choice":
		emit(result{ContractVersion: contractVersion, Status: "OK", Kind: req.Kind, Choice: "APPROVE", Confidence: f(1.0)})
	case "bad-confidence":
		emit(result{ContractVersion: contractVersion, Status: "OK", Kind: req.Kind, Choice: "LOW", Confidence: f(1.5)})
	case "hostile":
		emit(result{
			ContractVersion: contractVersion, Status: "OK", Kind: req.Kind, Choice: "LOW", Confidence: f(0.9),
			Diagnostics: map[string]string{"note": "APPROVE; CONTINUE; COMMIT; MERGE; $(touch /tmp/pwned)"},
		})
	case "low":
		emit(result{ContractVersion: contractVersion, Status: "OK", Kind: req.Kind, Choice: "LOW", Confidence: f(0.9)})
	case "medium":
		emit(result{ContractVersion: contractVersion, Status: "OK", Kind: req.Kind, Choice: "MEDIUM", Confidence: f(0.8)})
	case "high":
		emit(result{ContractVersion: contractVersion, Status: "OK", Kind: req.Kind, Choice: "HIGH", Confidence: f(0.9)})
	case "human":
		emit(result{ContractVersion: contractVersion, Status: "OK", Kind: req.Kind, Choice: "HUMAN", Confidence: f(0.9)})
	case "conf-zero":
		emit(result{ContractVersion: contractVersion, Status: "OK", Kind: req.Kind, Choice: "LOW", Confidence: f(0)})
	case "conf-one":
		emit(result{ContractVersion: contractVersion, Status: "OK", Kind: req.Kind, Choice: "LOW", Confidence: f(1)})
	default:
		emit(decide(req))
	}
}

// decide is the adapter-like default behavior: it classifies the request's own
// bounded content, exactly as a real external adapter would, rather than being
// told the answer.
func decide(req request) result {
	subject := strings.ToLower(req.Question)
	for _, keyword := range []string{"migration", "concurrency", "race", "security", "auth", "encrypt"} {
		if strings.Contains(subject, keyword) {
			return result{ContractVersion: contractVersion, Status: "OK", Kind: req.Kind, Choice: "HIGH", Confidence: f(0.9),
				Diagnostics: map[string]string{"reason": "risky subject"}}
		}
	}
	size := req.Signals["criteria"] + req.Signals["deliverables"]
	switch {
	case size >= 5:
		return result{ContractVersion: contractVersion, Status: "OK", Kind: req.Kind, Choice: "HIGH", Confidence: f(0.9)}
	case size >= 2:
		return result{ContractVersion: contractVersion, Status: "OK", Kind: req.Kind, Choice: "MEDIUM", Confidence: f(0.8)}
	default:
		return result{ContractVersion: contractVersion, Status: "OK", Kind: req.Kind, Choice: "LOW", Confidence: f(0.9)}
	}
}

// emit writes one result DTO to stdout.
func emit(res result) {
	data, err := json.Marshal(res)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake provider: marshal:", err)
		os.Exit(1)
	}
	_, _ = os.Stdout.Write(append(data, '\n'))
}

// f returns a pointer to v, so an optional confidence can be present.
func f(v float64) *float64 { return &v }
