package config

import (
	"strings"
	"testing"
)

func evalEnabled() *bool {
	enabled := true
	return &enabled
}

func validLocalEval() SecurityEvalConfig {
	return SecurityEvalConfig{
		Enabled:          evalEnabled(),
		Profile:          SecurityEvalProfileLocal,
		Target:           "local-model",
		PromptfooVersion: defaultSecurityEvalPromptfooVersion,
		AttackProvider:   "local-attack",
		GradingProvider:  "local-grading",
		TargetProvider:   "local-target",
		ConfigPath:       "evals/promptfoo.yaml",
		DataHandling:     SecurityEvalDataLocalOnly,
		SeverityMode:     SecurityEvalSeverityNone,
	}
}

func TestValidateSecurityEvalRequiresLocalProviders(t *testing.T) {
	e := validLocalEval()
	e.TargetProvider = ""
	if err := validateSecurityEval(SecurityConfig{Evaluation: e}); err == nil || !strings.Contains(err.Error(), "target_provider") {
		t.Fatalf("err = %v, want a target_provider error", err)
	}
}

func TestValidateSecurityEvalRemoteRequiresAcknowledgement(t *testing.T) {
	e := validLocalEval()
	e.Profile = SecurityEvalProfileRemote
	e.DataHandling = SecurityEvalDataRemoteAllowed
	if err := validateSecurityEval(SecurityConfig{Evaluation: e}); err == nil || !strings.Contains(err.Error(), "remote_provider") {
		t.Fatalf("err = %v, want a remote_provider error", err)
	}
	e.RemoteProvider = true
	e.DataHandling = SecurityEvalDataLocalOnly
	if err := validateSecurityEval(SecurityConfig{Evaluation: e}); err == nil || !strings.Contains(err.Error(), "data_handling") {
		t.Fatalf("err = %v, want a data_handling error", err)
	}
	e.DataHandling = SecurityEvalDataRemoteAllowed
	if err := validateSecurityEval(SecurityConfig{Evaluation: e}); err != nil {
		t.Fatalf("remote acknowledgement rejected: %v", err)
	}
}

func TestValidateSecurityEvalRejectsUnsafeConfiguration(t *testing.T) {
	cases := map[string]func(*SecurityEvalConfig){
		"pinned version":   func(e *SecurityEvalConfig) { e.PromptfooVersion = "9.9.9" },
		"unknown severity": func(e *SecurityEvalConfig) { e.SeverityMode = "cvss" },
		"escaping path":    func(e *SecurityEvalConfig) { e.ConfigPath = "../outside.yaml" },
		"negative limit":   func(e *SecurityEvalConfig) { e.RequestLimit = -1 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			e := validLocalEval()
			mutate(&e)
			if err := validateSecurityEval(SecurityConfig{Evaluation: e}); err == nil {
				t.Fatal("invalid evaluation configuration was accepted")
			}
		})
	}
}

func TestValidateSecurityEvalAcceptsValidLocal(t *testing.T) {
	if err := validateSecurityEval(SecurityConfig{Evaluation: validLocalEval()}); err != nil {
		t.Fatalf("valid local evaluation rejected: %v", err)
	}
}
