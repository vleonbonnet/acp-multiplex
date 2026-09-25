package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// TestAgentTreeDiesWithProxy hard-kills the proxy, which Windows does not
// propagate to children, and checks that the job object still takes down
// a child of the agent that would otherwise run forever.
func TestAgentTreeDiesWithProxy(t *testing.T) {
	proxy, _, _, dir := startProxy(t, "ACP_MULTIPLEX_TEST_SLEEPER=1")

	var pid int
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		b, err := os.ReadFile(filepath.Join(dir, "sleeper.pid"))
		if err == nil {
			if pid, err = strconv.Atoi(string(b)); err != nil {
				t.Fatalf("bad sleeper pid %q", b)
			}
			break
		}
		if time.Now().After(deadline) {
			logProxy(t, dir, proxy.Process.Pid)
			t.Fatal("agent did not start its child")
		}
	}
	h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		t.Fatalf("open agent's child: %v", err)
	}
	defer windows.CloseHandle(h)
	defer windows.TerminateProcess(h, 1) // don't leak it if the test fails

	if err := proxy.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	proxy.Wait()

	if ev, err := windows.WaitForSingleObject(h, 10000); ev != windows.WAIT_OBJECT_0 {
		t.Fatalf("agent's child survived the proxy: wait=%#x, %v", ev, err)
	}
}
