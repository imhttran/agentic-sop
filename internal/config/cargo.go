package config

import "fmt"

// Cargo is the optional, disabled-by-default Cargo permission block (SOP-001).
//
// It carries two INDEPENDENT permissions, one per execution path:
//
//   - agent_tool: lets the agent tool harness (internal/toolharness) run a
//     supported Cargo command through internal/commandpolicy.
//   - acceptance_check: lets plan acceptance checks (internal/cli) run a
//     supported Cargo command through internal/commandpolicy.
//
// Each permission is a *bool so an omitted value is distinguishable from an
// explicit false, and both resolve to DISABLED. The block is additive: a
// project that has no `cargo` block keeps working unchanged, and enabling one
// permission never enables the other. A permission is necessary but not
// sufficient: internal/commandpolicy still classifies the command and rejects
// unsupported subcommands/arguments, shell syntax, and a --manifest-path that
// resolves outside the repository BEFORE execution.
type Cargo struct {
	// AgentTool enables Cargo commands for the agent tool harness. Off by default.
	AgentTool *bool `yaml:"agent_tool"`
	// AcceptanceCheck enables Cargo commands for plan acceptance checks. Off by
	// default. It is independent of AgentTool.
	AcceptanceCheck *bool `yaml:"acceptance_check"`
}

// CargoAgentToolEnabled reports whether the agent tool harness may run supported
// Cargo commands. An omitted flag and an explicit false both resolve to disabled.
func (c Config) CargoAgentToolEnabled() bool {
	return c.Cargo.AgentTool != nil && *c.Cargo.AgentTool
}

// CargoAcceptanceCheckEnabled reports whether plan acceptance checks may run
// supported Cargo commands. It is independent of the agent tool permission, and
// an omitted flag and an explicit false both resolve to disabled.
func (c Config) CargoAcceptanceCheckEnabled() bool {
	return c.Cargo.AcceptanceCheck != nil && *c.Cargo.AcceptanceCheck
}

// validateCargo validates the Cargo permission block. Every field is an optional
// boolean, so there is nothing to reject today; the hook keeps a focused error
// available if the block grows.
func (c *Config) validateCargo() error {
	return nil
}

// applyLegacyCargo maps the deprecated spellings onto the canonical cargo block.
//
// Precedence and conflict rules:
//   - an omitted canonical value takes the legacy value;
//   - canonical and legacy values that agree are accepted;
//   - canonical and legacy values that disagree are an error (never silently
//     choose one, because that could widen or remove a permission);
//   - neither value means the default, false.
//
// The mapping is intentionally one-way: the legacy fields are never written
// back, and a new configuration should use cargo.agent_tool /
// cargo.acceptance_check directly.
func (c *Config) applyLegacyCargo() error {
	if c.Agent.ToolCargo != nil {
		if c.Cargo.AgentTool != nil && *c.Cargo.AgentTool != *c.Agent.ToolCargo {
			return fmt.Errorf("config: agent.tool_cargo=%t conflicts with cargo.agent_tool=%t; remove the legacy key or make them agree", *c.Agent.ToolCargo, *c.Cargo.AgentTool)
		}
		if c.Cargo.AgentTool == nil {
			c.Cargo.AgentTool = c.Agent.ToolCargo
		}
	}
	if c.Validation.AcceptanceChecks.Cargo != nil {
		if c.Cargo.AcceptanceCheck != nil && *c.Cargo.AcceptanceCheck != *c.Validation.AcceptanceChecks.Cargo {
			return fmt.Errorf("config: validation.acceptance_checks.cargo=%t conflicts with cargo.acceptance_check=%t; remove the legacy key or make them agree", *c.Validation.AcceptanceChecks.Cargo, *c.Cargo.AcceptanceCheck)
		}
		if c.Cargo.AcceptanceCheck == nil {
			c.Cargo.AcceptanceCheck = c.Validation.AcceptanceChecks.Cargo
		}
	}
	return nil
}
