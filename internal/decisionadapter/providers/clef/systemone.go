package clef

// This file defines the SystemOne wire types exchanged with Ollama's
// /v1/systemone endpoint, the CLEF-002-selected transport. These types are
// deliberately unexported and are confined to the Clef provider: they never
// appear in the public decision package.
//
// Only the SystemOne (/v1/systemone) transport is implemented. The unverified
// MLX/oMLX clef-4bit runtime is NOT represented here: it is not enabled.

// systemOneRequest is the body of POST /v1/systemone.
type systemOneRequest struct {
	Model     string                       `json:"model"`
	State     string                       `json:"state,omitempty"`
	Questions map[string]systemOneQuestion `json:"questions"`
}

// systemOneQuestion is a single question in a SystemOne request. The Type field
// uses SystemOne terminology: "choice", "score", or "noul".
//
// Criteria is polymorphic because SystemOne requires a different JSON shape per
// question type: a CHOICE question must send an object mapping each allowed
// option key to a description or null ({...: null}), while a SCORE question must
// send an array of 2-26 candidate descriptions.
type systemOneQuestion struct {
	Type         string   `json:"type"`
	Criteria     any      `json:"criteria,omitempty"`
	Choices      []string `json:"choices,omitempty"`
	Instructions string   `json:"instructions,omitempty"`
}

// systemOneResponse is the body of a successful /v1/systemone response.
type systemOneResponse struct {
	Model   string                     `json:"model"`
	Answers map[string]systemOneAnswer `json:"answers"`
	Usage   systemOneUsage             `json:"usage"`
}

// systemOneAnswer is a single answer keyed by question ID.
type systemOneAnswer struct {
	Type          string             `json:"type"`
	Choice        string             `json:"choice,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Confidence    *float64           `json:"confidence,omitempty"`
	Noul          *float64           `json:"noul,omitempty"`
	Score         *float64           `json:"score,omitempty"`
}

// systemOneUsage mirrors the token accounting SystemOne reports.
type systemOneUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}
