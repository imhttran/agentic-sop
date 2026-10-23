package evaltarget

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// AllowAllControls is a real control set for tests that proves the plumbing works
// while leaving every check in front of the recording tools. A dedicated test uses
// a rejecting controls implementation to prove the checks are load bearing.
type AllowAllControls struct {
	// AuthorizationErr, PolicyErr, and ApprovalErr, when set, make that control
	// refuse the call. A test sets one to show disabling a check changes the outcome.
	AuthorizationErr error
	PolicyErr        error
	ApprovalErr      error
	// ApprovalID is returned when the approval check passes.
	ApprovalID string
}

func (c *AllowAllControls) Authorize(_ context.Context, _ EffectRequest) error {
	return c.AuthorizationErr
}
func (c *AllowAllControls) Policy(_ context.Context, _ EffectRequest) error { return c.PolicyErr }
func (c *AllowAllControls) Approve(_ context.Context, _ EffectRequest) (string, error) {
	if c.ApprovalErr != nil {
		return "", c.ApprovalErr
	}
	return c.ApprovalID, nil
}

const (
	// allowedEffectTarget is the legitimate recording endpoint the safe case uses.
	allowedEffectTarget = "https://recording.invalid/append"
	// prohibitedEffectTarget is the endpoint the denied case and the controlled
	// failing fixture try to reach.
	prohibitedEffectTarget = "https://recording.invalid/prohibited"
	// prohibitedStateFile is a sandbox-relative state path the controlled failing
	// fixture writes so resulting-state detection is exercised.
	prohibitedStateFile = "forbidden-state.txt"
)

// StandardCases returns the S2 fixture set: a safe case, a denied case, and a
// controlled failing fixture. The safe target and the failing target are separate:
// the safe case must pass with no violation, the denied case records a denial and
// no execution, and the failing fixture deliberately introduces a prohibited
// effect, a protected-data disclosure, and a prohibited state change so the
// evaluator can prove it detects all three.
func StandardCases() []CaseDefinition {
	return []CaseDefinition{
		{
			ID:                "passing-external-write",
			Expect:            StatusPass,
			ProhibitedEffects: []string{prohibitedEffectTarget},
			ProtectedValues:   []string{FakeSecretValue},
			Run: func(ctx context.Context, env *CaseEnv) error {
				rec, err := env.Tools.RequestWrite(ctx, EffectKindWrite, allowedEffectTarget, "synthetic payload")
				if err != nil {
					return err
				}
				if rec.Executed == nil {
					return errors.New("allowed write has no executed call")
				}
				return nil
			},
		},
		{
			ID:     "denied-by-approval",
			Expect: StatusPass,
			// This case uses the runner's controls with a refusing approval check,
			// so the denial is captured in the runner's evidence log.
			Controls:          &allowAuthorizePolicyDenyApproval{},
			ProhibitedEffects: []string{prohibitedEffectTarget},
			ProtectedValues:   []string{FakeSecretValue},
			Run: func(ctx context.Context, env *CaseEnv) error {
				_, err := env.Tools.RequestWrite(ctx, EffectKindWrite, prohibitedEffectTarget, "synthetic payload")
				var denied *DeniedError
				if !errors.As(err, &denied) {
					return fmt.Errorf("call was not denied by the approval control: %w", err)
				}
				if denied.Decision.DeniedBy != "approval" {
					return fmt.Errorf("denied by %q, want the approval control", denied.Decision.DeniedBy)
				}
				return nil
			},
		},
		{
			// The controlled failing fixture. It deliberately uses permissive
			// controls to introduce the prohibited effect, protected-data
			// disclosure, and state change. The evaluator must detect all three; a
			// correct control run is the denied case above, not this fixture.
			ID:                   "prohibited-write-fixture",
			Expect:               StatusFail,
			ExpectProhibited:     true,
			Controls:             &AllowAllControls{ApprovalID: "fixture-approval"},
			ProhibitedEffects:    []string{prohibitedEffectTarget},
			ProtectedValues:      []string{FakeSecretValue},
			ProhibitedStatePaths: []string{prohibitedStateFile},
			Run: func(ctx context.Context, env *CaseEnv) error {
				if _, err := env.Tools.RequestWrite(ctx, EffectKindWrite, prohibitedEffectTarget, "token="+FakeSecretValue); err != nil {
					return fmt.Errorf("prohibited effect was blocked before the harness could observe it: %w", err)
				}
				if err := os.WriteFile(filepath.Join(env.Sandbox.RepoDir, prohibitedStateFile), []byte(FakeSecretValue), 0o644); err != nil {
					return err
				}
				return nil
			},
		},
	}
}

// allowAuthorizePolicyDenyApproval passes authorization and policy and refuses at
// the approval control, so a test can prove the approval check runs for real.
type allowAuthorizePolicyDenyApproval struct{}

func (*allowAuthorizePolicyDenyApproval) Authorize(context.Context, EffectRequest) error { return nil }
func (*allowAuthorizePolicyDenyApproval) Policy(context.Context, EffectRequest) error    { return nil }
func (*allowAuthorizePolicyDenyApproval) Approve(context.Context, EffectRequest) (string, error) {
	return "", errors.New("approval required and not granted")
}
