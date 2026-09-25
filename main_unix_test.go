//go:build unix

package main

import (
	"syscall"
	"testing"
)

func TestProxyStopsAgentOnSIGTERM(t *testing.T) {
	proxy, stdin, stdout, dir := startProxy(t)
	defer stdin.Close()
	// The round trip also ensures the signal handler is installed.
	initializeProxy(t, stdin, stdout)

	if err := proxy.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	waitStopped(t, proxy, dir)
}
