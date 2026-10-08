//go:build linux || darwin

package ipc

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

const helperEnv = "GRUNTLED_IPC_HELPER"

// TestMain turns the test binary into a daemon stand-in when helperEnv
// names a directory: lock, listen, serve, print "ready", then block on
// stdin until killed.
func TestMain(m *testing.M) {
	if dir := os.Getenv(helperEnv); dir != "" {
		os.Exit(runHelper(dir))
	}
	os.Exit(m.Run())
}

func runHelper(dir string) int {
	l, err := TryLock(filepath.Join(dir, LockName))
	if err != nil {
		fmt.Fprintln(os.Stderr, "helper lock:", err)
		return 2
	}
	defer l.Release()
	ln, err := Listen(filepath.Join(dir, SockName))
	if err != nil {
		fmt.Fprintln(os.Stderr, "helper listen:", err)
		return 2
	}
	info := Info{PID: os.Getpid(), Version: "helper"}
	s := Serve(ln, info, func() *Snapshot { return nil })
	defer s.Close()
	fmt.Println("ready")
	_, _ = io.Copy(io.Discard, os.Stdin)
	return 0
}

func TestCrashRecovery(t *testing.T) {
	dir := shortDir(t)
	lockPath := filepath.Join(dir, LockName)
	sock := filepath.Join(dir, SockName)

	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), helperEnv+"="+dir)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	killed := false
	defer func() {
		if !killed {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()

	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || line != "ready\n" {
		t.Fatalf("helper said %q, %v", line, err)
	}

	if _, err := TryLock(lockPath); !errors.Is(err, ErrHeld) {
		t.Fatalf("TryLock while helper runs: err = %v, want ErrHeld", err)
	}
	if held, err := Probe(lockPath); err != nil || !held {
		t.Fatalf("Probe while helper runs: held=%v err=%v", held, err)
	}
	info, _, err := Query(sock, OpPing, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if info.PID != cmd.Process.Pid {
		t.Fatalf("ping pid = %d, want %d", info.PID, cmd.Process.Pid)
	}

	if err := cmd.Process.Kill(); err != nil { // SIGKILL: no cleanup runs
		t.Fatal(err)
	}
	killed = true
	_ = cmd.Wait()

	if fi, err := os.Lstat(sock); err != nil || fi.Mode().Type() != fs.ModeSocket {
		t.Fatalf("expected a stale socket after SIGKILL: %v %v", fi, err)
	}
	if _, _, err := Query(sock, OpPing, time.Second); !errors.Is(err, ErrNoDaemon) {
		t.Fatalf("stale socket: err = %v, want ErrNoDaemon", err)
	}

	l, err := TryLock(lockPath)
	if err != nil {
		t.Fatalf("TryLock after SIGKILL: %v", err)
	}
	defer l.Release()
	ln, err := Listen(sock)
	if err != nil {
		t.Fatalf("Listen over stale socket: %v", err)
	}
	s := Serve(ln, sampleInfo(), func() *Snapshot { return nil })
	defer s.Close()
	if info, _, err := Query(sock, OpPing, time.Second); err != nil || info.PID != sampleInfo().PID {
		t.Fatalf("new daemon: info=%+v err=%v", info, err)
	}
}
