// Command sop-controller serves the local-first SOP dashboard.
package main

import (
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"

	assets "sop-controller"

	"sop-controller/internal/config"
	"sop-controller/internal/sopclient"
	"sop-controller/internal/web"
)

func main() {
	config.LoadEnvFiles()
	cfg := config.Load()

	// Merge explicit roots with projects discovered under SOP_CONTROLLER_WORKSPACES
	// (an allowlist, never the wider filesystem).
	report, err := config.ResolveReport(cfg.ProjectRoots, cfg.Workspaces, cfg.DiscoveryDepth)
	if err != nil {
		log.Fatalf("project discovery: %v", err)
	}
	for _, d := range report.Diagnostics {
		log.Printf("[discovery] %s: %s", d.Path, d.Reason)
	}
	roots := make([]string, 0, len(report.Projects))
	for _, p := range report.Projects {
		roots = append(roots, p.Root)
	}
	sop, err := sopclient.New(roots, cfg.SOPBin, cfg.CommandTimeout)
	if err != nil {
		log.Fatalf("SOP boundary: %v", err)
	}
	defer sop.Close()

	views, err := web.NewViews(assets.FS)
	if err != nil {
		log.Fatalf("parse templates: %v", err)
	}
	staticFS, err := fs.Sub(assets.FS, "static")
	if err != nil {
		log.Fatalf("static assets: %v", err)
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
		Discovery:      report,
	})

	// Bind explicitly so the startup summary can report the actual bound port: a
	// dynamically allocated ":0" port only exists once the listener is open.
	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		log.Fatalf("listen on %s: %v", cfg.Addr, err)
	}
	log.Printf("SOP Controller bound to %s (%d project(s), poll %s, attention %s)", ln.Addr(), len(roots), cfg.PollInterval, cfg.AttentionInterval)
	fmt.Print(web.StartupSummary(
		web.ResolveStartupURLs(ln.Addr(), "http", cfg.AllowNetwork, web.LANAddresses()),
		cfg.AllowNetwork,
	))
	if err := http.Serve(ln, handler); err != nil {
		log.Fatal(err)
	}
}
