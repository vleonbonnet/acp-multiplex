package main

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// TestMain lets the test binary stand in for the proxy, the agent and the
// agent's own child, so tests can run runProxy as a real process tree.
// Each role hands the next one its role through the environment.
func TestMain(m *testing.M) {
	switch os.Getenv("ACP_MULTIPLEX_TEST_HELPER") {
	case "proxy":
		os.Setenv("ACP_MULTIPLEX_TEST_HELPER", "agent")
		runProxy() // os.Args[1:] names this binary as the agent
	case "agent":
		helperAgent()
	case "sleeper":
		time.Sleep(time.Hour)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// helperAgent serves mockAgent on stdio and records in the test directory
// that it saw stdin EOF. With ACP_MULTIPLEX_TEST_SLEEPER set, it first
// starts a child that never exits on its own and records its pid.
func helperAgent() {
	dir := os.Getenv("ACP_MULTIPLEX_TEST_DIR")
	if os.Getenv("ACP_MULTIPLEX_TEST_SLEEPER") != "" {
		exe, err := os.Executable()
		if err != nil {
			os.Exit(1)
		}
		sleeper := exec.Command(exe)
		sleeper.Env = append(os.Environ(), "ACP_MULTIPLEX_TEST_HELPER=sleeper")
		if err := sleeper.Start(); err != nil {
			os.Exit(1)
		}
		// Rename so the test never reads a partial pid.
		tmp := filepath.Join(dir, "sleeper.pid.tmp")
		os.WriteFile(tmp, []byte(strconv.Itoa(sleeper.Process.Pid)), 0600)
		os.Rename(tmp, filepath.Join(dir, "sleeper.pid"))
	}
	mockAgent(os.Stdin, os.Stdout)
	os.WriteFile(filepath.Join(dir, "agent-eof"), nil, 0600)
	os.Exit(0)
}

// startProxy runs this test binary as `acp-multiplex <this binary>`, with
// the agent played by helperAgent. It returns the proxy, the primary
// frontend's end of its stdio, and the directory holding the socket, the
// proxy log and the files the agent writes.
func startProxy(t *testing.T, env ...string) (*exec.Cmd, io.WriteCloser, *bufio.Scanner, string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// Not t.TempDir: its long paths can exceed the unix socket path limit.
	dir, err := os.MkdirTemp("", "acpm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	cmd := exec.Command(exe, exe)
	cmd.Env = append(os.Environ(),
		"ACP_MULTIPLEX_TEST_HELPER=proxy",
		"ACP_MULTIPLEX_TEST_DIR="+dir,
		"XDG_RUNTIME_DIR="+dir)
	cmd.Env = append(cmd.Env, env...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill() })

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	return cmd, stdin, scanner, dir
}

// initializeProxy does an initialize round trip through the proxy, which
// shows that the proxy and the agent are up.
func initializeProxy(t *testing.T, stdin io.Writer, stdout *bufio.Scanner) {
	t.Helper()
	stdin.Write([]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1}}` + "\n"))
	var resp map[string]interface{}
	if err := json.Unmarshal(readLine(t, stdout, 10*time.Second), &resp); err != nil || resp["result"] == nil {
		t.Fatalf("bad initialize response: %v %v", resp, err)
	}
}

// waitStopped checks that the proxy exits cleanly, after stopping the
// agent through stdin EOF rather than killing it, and removes its socket.
func waitStopped(t *testing.T, proxy *exec.Cmd, dir string) {
	t.Helper()
	exited := make(chan error, 1)
	go func() { exited <- proxy.Wait() }()
	select {
	case err := <-exited:
		if err != nil {
			logProxy(t, dir, proxy.Process.Pid)
			t.Fatalf("proxy exited with %v", err)
		}
	case <-time.After(2 * agentStopTimeout):
		logProxy(t, dir, proxy.Process.Pid)
		t.Fatalf("proxy still running after %v", 2*agentStopTimeout)
	}

	if _, err := os.Stat(filepath.Join(dir, "agent-eof")); err != nil {
		logProxy(t, dir, proxy.Process.Pid)
		t.Errorf("agent did not see stdin EOF: %v", err)
	}
	sock := filepath.Join(dir, "acp-multiplex", strconv.Itoa(proxy.Process.Pid)+".sock")
	if _, err := os.Stat(sock); !os.IsNotExist(err) {
		t.Errorf("socket %s not removed: %v", sock, err)
	}
}

// logProxy dumps the proxy's log to help diagnose a failure.
func logProxy(t *testing.T, dir string, pid int) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "acp-multiplex", "logs", strconv.Itoa(pid)+".log"))
	if err != nil {
		t.Logf("proxy log: %v", err)
		return
	}
	t.Logf("proxy log:\n%s", b)
}

func TestProxyExitsWhenPrimaryDisconnects(t *testing.T) {
	proxy, stdin, stdout, dir := startProxy(t)
	initializeProxy(t, stdin, stdout)

	stdin.Close()
	waitStopped(t, proxy, dir)
}
