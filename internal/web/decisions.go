package web

import (
	"context"
	"net/http"

	"sop-controller/internal/sopclient"
)

// C2-006 decisions surface: a read-only projection of SOP-reported gates
// (TaskSummary.NeedsHuman) and pending changed tasks (ChangedTasks.Pending).
// Its controls POST to the existing approve/decline/accept-changed routes.

// decisionGate is one SOP-reported approval gate.
type decisionGate struct {
	TaskID string
	Title  string
	Kind   string
}

// decisionAccept is one changed task still pending accept.
type decisionAccept struct {
	TaskID        string
	Title         string
	Stage         string
	ChangeSummary string
}

// decisionsData feeds decisions.html and GET /projects/{project}/decisions.
type decisionsData struct {
	baseData
	Project sopclient.ProjectDetail
	Gates   []decisionGate
	Accepts []decisionAccept
	// GatesUnread is set when SOP did not report an approval listing for this
	// project. It is an absence, distinct from "no gates": the panel must not
	// claim there is no gate when the listing could not be read
	// (docs/specs/HUMAN-APPROVAL.md).
	GatesUnread bool
	// ChangedTasksError is set when the listing could not be read.
	ChangedTasksError string
}

// decisions projects from reads the caller already has; it makes no SOP call.
func decisions(base baseData, project sopclient.ProjectDetail, changed sopclient.ChangedTasks, changedErr string) decisionsData {
	d := decisionsData{
		baseData:          base,
		Project:           project,
		ChangedTasksError: changedErr,
		// An unreported listing is an absence, never "no gates".
		GatesUnread: !project.ApprovalsReported,
	}

	// A task appears only when SOP's approval listing reports an applicable gate.
	for _, t := range project.Tasks {
		if !t.NeedsHuman {
			continue
		}
		d.Gates = append(d.Gates, decisionGate{TaskID: t.ID, Title: t.Title, Kind: t.ApprovalKind})
	}

	// Accept: SOP's changed-executed tasks still pending, when the read succeeded.
	if changedErr == "" {
		for _, c := range changed.Pending() {
			d.Accepts = append(d.Accepts, decisionAccept{
				TaskID:        c.TaskID,
				Title:         c.Title,
				Stage:         c.Stage,
				ChangeSummary: c.ChangeSummary,
			})
		}
	}
	return d
}

// projectDecisions renders the decisions surface; GET is side-effect free.
func (h *Handlers) projectDecisions(w http.ResponseWriter, r *http.Request) {
	detail, err := h.sop.Project(r.Context(), r.PathValue("project"))
	if err != nil {
		h.renderError(w, r, err)
		return
	}
	changed, changedErr := h.readChangedTasks(r.Context(), detail)
	h.render(w, http.StatusOK, "decisions.html", decisions(
		h.base(r, "projects"), detail, changed, changedErr))
}

// readChangedTasks returns the listing, or an error string instead of an empty
// success. Shared by the project page and decisions route.
func (h *Handlers) readChangedTasks(ctx context.Context, detail sopclient.ProjectDetail) (sopclient.ChangedTasks, string) {
	changed, err := h.sop.ChangedTasks(ctx, detail.Summary.ID)
	if err != nil {
		return sopclient.ChangedTasks{}, err.Error()
	}
	return changed, ""
}
