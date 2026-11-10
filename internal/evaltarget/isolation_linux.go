//go:build linux

package evaltarget

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
)

func isolationAvailable() bool {
	_, err := exec.LookPath("bwrap")
	return err == nil
}

func isolationKind() string { return "bwrap" }

// isolationCommand runs argv under bubblewrap with a private mount and network
// namespace. Only the case root is writable; the rest of the system is read-only.
func isolationCommand(ctx context.Context, root, dir string, argv, env []string) (*exec.Cmd, error) {
	bwrap, err := exec.LookPath("bwrap")
	if err != nil {
		return nil, ErrIsolationUnavailable
	}
	cmd := exec.CommandContext(ctx, bwrap, bwrapArgs(root, dir, argv)...)
	cmd.Dir = dir
	cmd.Env = env
	return cmd, nil
}

// liveSocketPrefix is the prefix of the Unix domain socket the Linux live
// sandbox uses to reach the fixed endpoint. Each invocation uses a unique path
// in the writable case root, so an asynchronously closing earlier listener can
// never unlink a replacement socket.
const liveSocketPrefix = ".sop-live-"

var liveSocketSeq atomic.Uint64

func liveSocketPath(root string) string {
	return filepath.Join(root, fmt.Sprintf("%s%d-%d.sock", liveSocketPrefix, os.Getpid(), liveSocketSeq.Add(1)))
}

// liveIsolationCommand is the opt-in, evaluation-only live variant. It keeps the
// same private mount, environment, and process namespace as isolationCommand and
// additionally starts a host-side proxy on a Unix domain socket inside the case
// root. The sandbox has no network route; the socket is the only channel, and the
// proxy dials exactly LiveEndpointDialAddress. This is the Linux equivalent of the
// macOS single-rule Seatbelt profile: host networking is never shared.
//
// The caller must cancel ctx to release the proxy listener and its connections.
func liveIsolationCommand(ctx context.Context, root, dir string, argv, env []string) (*exec.Cmd, error) {
	bwrap, err := exec.LookPath("bwrap")
	if err != nil {
		return nil, ErrIsolationUnavailable
	}
	socketPath := liveSocketPath(root)
	// The root is the caller's disposable case directory. Removing a stale socket
	// at this unique path prevents EADDRINUSE without touching any other path.
	_ = os.Remove(socketPath)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("evaltarget: create live endpoint socket: %w", err)
	}
	go serveLiveEndpoint(ctx, listener)

	// Append the socket path to a copy of the caller's environment. The live
	// decider reads it; the sandboxed process has no other route to the endpoint.
	env = append(append([]string(nil), env...), LiveSocketEnv+"="+socketPath)

	cmd := exec.CommandContext(ctx, bwrap, bwrapArgs(root, dir, argv)...)
	cmd.Dir = dir
	cmd.Env = env
	return cmd, nil
}

// bwrapArgs builds the shared bubblewrap argument list. --unshare-all includes a
// private network namespace, so neither the default nor the live sandbox can
// reach a host loopback port directly.
func bwrapArgs(root, dir string, argv []string) []string {
	args := []string{
		"--die-with-parent",
		"--unshare-all",
		"--new-session",
		"--proc", "/proc",
		"--dev", "/dev",
		"--bind", root, root,
		"--chdir", dir,
	}
	for _, ro := range []string{"/usr", "/bin", "/sbin", "/lib", "/lib64", "/etc"} {
		if _, err := filepath.EvalSymlinks(ro); err == nil {
			args = append(args, "--ro-bind", ro, ro)
		}
	}
	args = append(args, "--")
	args = append(args, argv...)
	return args
}

// serveLiveEndpoint forwards each accepted Unix-socket connection to the single
// allowlisted endpoint. It closes the listener when ctx ends. No other
// destination is ever dialed.
func serveLiveEndpoint(ctx context.Context, listener net.Listener) {
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go proxyLiveEndpoint(ctx, conn)
	}
}

// proxyLiveEndpoint copies bytes between one sandbox connection and one
// connection to the fixed endpoint. It closes both sides when either finishes or
// when the run context ends, so a canceled campaign leaves no forwarding
// goroutine or upstream connection behind.
func proxyLiveEndpoint(ctx context.Context, client net.Conn) {
	defer func() { _ = client.Close() }()
	var dialer net.Dialer
	upstream, err := dialer.DialContext(ctx, "tcp", LiveEndpointDialAddress)
	if err != nil {
		return
	}
	defer func() { _ = upstream.Close() }()

	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(upstream, client)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(client, upstream)
		done <- struct{}{}
	}()

	select {
	case <-ctx.Done():
	case <-done:
	}
	_ = client.Close()
	_ = upstream.Close()
	<-done
}
