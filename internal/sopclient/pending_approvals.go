package sopclient

import "context"

// Cross-project approval inbox projection. `sop approvals --json` is the only
// source of gate presence and applicability (see approvals.go); this file only
// aggregates that listing across the observed projects so a single view can
// show every gate without the controller deciding that any gate exists. It is a
// pure read: no gate is reconstructed from task status, prose, or attempts, and
// no approval state is written locally.

// ProjectApprovals is one project's applicable approval gates as SOP reports
// them. It carries the project identity so a combined view can attribute each
// gate and delegate to the project-scoped command routes.
type ProjectApprovals struct {
	ProjectID   string
	ProjectName string
	// Reported is false when SOP did not report a listing for this project (its
	// `sop approvals --json` failed or was unparsable). Reported=false is an
	// absence, never "no gates"; the two must not be conflated.
	Reported bool
	// Entries are SOP's applicable gates, in SOP's order. Resolved/stale gates
	// are omitted: SOP must affirmatively report the gate open.
	Entries []ApprovalEntry
}

// PendingApprovals aggregates SOP's applicable approval gates across every
// observed project. It runs `sop approvals --json` once per project (a pure
// read) and keeps only entries SOP reports as applicable. A project whose
// listing read fails is returned with Reported=false rather than an empty
// success, so a failing SOP is never indistinguishable from a quiet one. The
// order follows the client's project order.
func (c *Client) PendingApprovals(ctx context.Context) []ProjectApprovals {
	out := make([]ProjectApprovals, 0, len(c.order))
	for _, id := range c.order {
		st := c.stores[id]
		name, _ := st.projectMeta()
		if name == "" {
			name = id
		}
		pa := ProjectApprovals{ProjectID: id, ProjectName: name}

		listing, err := c.Approvals(ctx, id)
		if err != nil || !listing.Reported {
			// An unread/absent listing is an absence, not "no gates".
			out = append(out, pa)
			continue
		}
		pa.Reported = true
		for _, e := range listing.Entries {
			if e.Applicable {
				pa.Entries = append(pa.Entries, e)
			}
		}
		out = append(out, pa)
	}
	return out
}
