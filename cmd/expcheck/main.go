// Command expcheck is the operator front end of internal/expharness for the
// independent design-review experiment. It starts no inference by itself.
//
//	expcheck preflight --dir PROJECT --providers ollama[,..] [--allow-cloud] [--allow-env A,B] [--used-classes medium] [--cutoff SHA]
//	expcheck proxy --upstream URL --max-calls N --max-tokens N --log FILE [--listen 127.0.0.1:0] [--allow-nonlocal-upstream]
//	expcheck attribute --run-dir DIR --dir PROJECT --records FILE [--audit FILE] (same policy flags as preflight)
//
// preflight and attribute read the CURRENT environment as the run environment, so run
// them in the same `env -i ...` as the sop call. proxy logs one run's provider traffic
// and enforces hard call and token ceilings; stop it with SIGINT to write the log.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/imhttran/agentic-sop/internal/expharness"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: expcheck preflight|proxy|attribute ...")
		os.Exit(2)
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	dir := fs.String("dir", ".", "project directory")
	providers := fs.String("providers", "", "approved providers, comma separated")
	allowCloud := fs.Bool("allow-cloud", false, "operator decision recorded: classes may be cloud-hosted")
	allowEnv := fs.String("allow-env", "SOP_OLLAMA_BASE_URL,SOP_MLX_BASE_URL,SOP_TOOL_AUDIT_LOG", "SOP_* variables that may be set")
	used := fs.String("used-classes", "medium", "--model-class values the experiment passes")
	cutoff := fs.String("cutoff", "", "required HEAD commit (history cutoff)")
	upstream := fs.String("upstream", "", "proxy: provider base URL")
	maxCalls := fs.Int("max-calls", 0, "proxy: hard ceiling on forwarded requests (required)")
	maxTokens := fs.Int64("max-tokens", 0, "proxy: hard ceiling on reported tokens (required)")
	logFile := fs.String("log", "", "proxy: records file written on SIGINT")
	listen := fs.String("listen", "127.0.0.1:0", "proxy: listen address")
	nonlocal := fs.Bool("allow-nonlocal-upstream", false, "proxy: permit a non-loopback upstream (needs operator authorization)")
	runDir := fs.String("run-dir", "", "attribute: the run directory")
	records := fs.String("records", "", "attribute: proxy records file")
	audit := fs.String("audit", "", "attribute: SOP_TOOL_AUDIT_LOG file used for the run")
	_ = fs.Parse(os.Args[2:])

	pol := expharness.Policy{AllowedProviders: split(*providers), AllowCloud: *allowCloud,
		AllowedEnv: split(*allowEnv), UsedClasses: split(*used), CutoffCommit: *cutoff}
	switch os.Args[1] {
	case "preflight":
		rep := expharness.Preflight(*dir, os.Environ(), pol)
		emit(rep)
		if !rep.OK() {
			os.Exit(1)
		}
	case "attribute":
		var recs []expharness.Record
		raw, err := os.ReadFile(*records)
		if err == nil {
			err = json.Unmarshal(raw, &recs)
		}
		die(err)
		a, err := expharness.Attribute(*runDir, expharness.Preflight(*dir, os.Environ(), pol), recs, *audit)
		die(err)
		emit(a)
		if len(a.Problems) > 0 {
			os.Exit(1)
		}
	case "proxy":
		if *maxCalls <= 0 || *maxTokens <= 0 || *upstream == "" || *logFile == "" {
			die(fmt.Errorf("proxy needs --upstream, --max-calls, --max-tokens and --log"))
		}
		u, err := url.Parse(*upstream)
		die(err)
		if h := u.Hostname(); !*nonlocal && h != "127.0.0.1" && h != "localhost" && h != "::1" {
			die(fmt.Errorf("upstream %s is not loopback; pass --allow-nonlocal-upstream only with operator authorization", h))
		}
		px := expharness.NewProxy(*upstream, *maxCalls, *maxTokens)
		ln, err := net.Listen("tcp", *listen)
		die(err)
		fmt.Println("listening", ln.Addr())
		go func() { _ = http.Serve(ln, px) }()
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		b, _ := json.MarshalIndent(px.Records(), "", "  ")
		die(os.WriteFile(*logFile, b, 0o600))
	default:
		die(fmt.Errorf("unknown subcommand %q", os.Args[1]))
	}
}

func split(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func emit(v any) { b, _ := json.MarshalIndent(v, "", "  "); fmt.Println(string(b)) }

func die(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "expcheck:", err)
		os.Exit(1)
	}
}
