// Package controller is the SOP Controller service entry point, shared by the
// unified `sop controller serve` command and the legacy `sop-controller`
// executable.
//
// SOP remains the workflow authority: this package serves a read-only dashboard
// over SOP's own state and delegates commands to the installed `sop` executable.
// It never owns lifecycle state.
package controller

import (
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"

	"github.com/imhttran/agentic-sop/internal/controller/assets"
	"github.com/imhttran/agentic-sop/internal/controller/config"
	"github.com/imhttran/agentic-sop/internal/controller/sopclient"
	"github.com/imhttran/agentic-sop/internal/controller/web"
)

// Serve starts the controller dashboard and serves until the listener closes. It
// mirrors the original sop-controller entry point: load configuration, resolve the
// allow-listed project roots, build the read-only SOP client and web server, bind
// explicitly (so the startup summary can report the bound port), and serve. Normal
// output (the startup summary) goes to stdout; diagnostics go to stderr. An error
// is returned to the caller rather than exiting the process, so a command layer can
// report it without ending the process under test.
func Serve(stdout, stderr io.Writer) error {
	logger := log.New(stderr, "", log.LstdFlags)

	config.LoadEnvFiles()
	cfg := config.Load()

	// Merge explicit roots with projects discovered under the configured workspaces
	// (an allowlist, never the wider filesystem).
	report, err := config.ResolveReport(cfg.ProjectRoots, cfg.Workspaces, cfg.DiscoveryDepth)
	if err != nil {
		return fmt.Errorf("project discovery: %w", err)
	}
	for _, d := range report.Diagnostics {
		logger.Printf("[discovery] %s: %s", d.Path, d.Reason)
	}
	roots := make([]string, 0, len(report.Projects))
	for _, p := range report.Projects {
		roots = append(roots, p.Root)
	}

	sop, err := sopclient.New(roots, cfg.SOPBin, cfg.CommandTimeout)
	if err != nil {
		return fmt.Errorf("SOP boundary: %w", err)
	}
	defer sop.Close()

	views, err := web.NewViews(assets.FS)
	if err != nil {
		return fmt.Errorf("parse templates: %w", err)
	}
	staticFS, err := fs.Sub(assets.FS, "static")
	if err != nil {
		return fmt.Errorf("static assets: %w", err)
	}

	handler := web.NewServer(web.Options{
		SOP:            sop,
		Views:          views,
		StaticFS:       staticFS,
		Poll:           cfg.PollInterval,
		Attention:      cfg.AttentionInterval,
		CommandTimeout: cfg.CommandTimeout,
		AllowNetwork:   cfg.AllowNetwork,
		AccessToken:    cfg.AccessToken,
		SessionTTL:     cfg.SessionTTL,
		SessionFile:    cfg.SessionFile,
		Operator:       cfg.Operator,
		Discovery:      report,
	})

	// Bind explicitly so the startup summary can report the actual bound port: a
	// dynamically allocated ":0" port only exists once the listener is open.
	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.Addr, err)
	}
	logger.Printf("SOP Controller bound to %s (%d project(s), poll %s, attention %s)", ln.Addr(), len(roots), cfg.PollInterval, cfg.AttentionInterval)

	session := web.SessionInfo{
		Lifetime: web.EffectiveSessionTTL(cfg.SessionTTL),
		File:     cfg.SessionFile,
	}
	if session.File != "" && !web.SessionStoreReady(session.File) {
		session.PersistError = "store not writable"
	}
	fmt.Fprint(stdout, web.StartupSummary(
		web.ResolveStartupURLs(ln.Addr(), "http", cfg.AllowNetwork, web.LANAddresses()),
		cfg.AllowNetwork,
		session,
	))

	return http.Serve(ln, handler)
}
