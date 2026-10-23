package evaltarget

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/approval"
	"github.com/imhttran/agentic-sop/internal/commandpolicy"
	"github.com/imhttran/agentic-sop/internal/domain"
)

// ProductionControls delegates the acceptance suite to SOP's real approval and
// command-policy boundaries. It contains no approval or policy rules of its own:
// approval state comes from approval.Service, and command safety comes from
// commandpolicy. The integration test supplies an isolated store and run
// directory; external effects still go through RecordingEffect.
type ProductionControls struct {
	Approvals *approval.Service
	TaskID    string
}

// Authorize requires an approved request for this task whose target matches the
// effect target. An unapproved, missing, or differently-scoped request refuses.
func (c *ProductionControls) Authorize(_ context.Context, req EffectRequest) error {
	view, err := c.view()
	if err != nil {
		return err
	}
	if !view.Present {
		return errors.New("no approval request exists for this task")
	}
	if view.Status != domain.ApprovalApproved {
		return fmt.Errorf("approval status is %s, not APPROVED", view.Status)
	}
	if strings.TrimSpace(view.Target) != strings.TrimSpace(req.Target) {
		return fmt.Errorf("approved target %q does not authorize %q", view.Target, req.Target)
	}
	return nil
}

// Policy delegates to SOP's production command policy. A RequiresApproval or
// Denied command is refused before the effect boundary.
func (c *ProductionControls) Policy(_ context.Context, req EffectRequest) error {
	if len(req.Command) == 0 {
		return errors.New("no command-policy evidence supplied")
	}
	class, err := commandpolicy.Classify(req.Command)
	if err != nil {
		return err
	}
	if class != commandpolicy.Safe {
		return fmt.Errorf("command policy classifies %q as %s", strings.Join(req.Command, " "), class)
	}
	return nil
}

// Approve returns the approved request identity for the matching target.
func (c *ProductionControls) Approve(_ context.Context, req EffectRequest) (string, error) {
	view, err := c.view()
	if err != nil {
		return "", err
	}
	if !view.Present || view.Status != domain.ApprovalApproved {
		return "", fmt.Errorf("no approved request for task %s", c.TaskID)
	}
	if strings.TrimSpace(view.Target) != strings.TrimSpace(req.Target) {
		return "", fmt.Errorf("approved target %q does not authorize %q", view.Target, req.Target)
	}
	return fmt.Sprintf("%s@%s", view.TaskID, view.RequestedAt.UTC().Format(time.RFC3339)), nil
}

func (c *ProductionControls) view() (approval.View, error) {
	if c == nil || c.Approvals == nil {
		return approval.View{}, errors.New("production approval boundary is unavailable")
	}
	if strings.TrimSpace(c.TaskID) == "" {
		return approval.View{}, errors.New("production approval requires a task id")
	}
	return c.Approvals.Approval(c.TaskID)
}
