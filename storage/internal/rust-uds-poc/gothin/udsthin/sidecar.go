package udsthin

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Sidecar manages the gcs-core-server process for this machine+user.
//
// Policy (one of several possible; chosen to be measurable):
//   - socket lives in a 0700 per-user directory ($XDG_RUNTIME_DIR or /tmp);
//   - if something already answers on the socket, reuse it (shared sidecar);
//   - otherwise spawn, wait for the "READY" line on stdout, then connect;
//   - the server exits itself after IdleSecs without RPCs and on parent death.
type Sidecar struct {
	Socket string
	PID    int
	cmd    *exec.Cmd
	owned  bool
	mu     sync.Mutex
}

var spawnMu sync.Mutex // serialise spawns within this process

func defaultSocket() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = filepath.Join(os.TempDir(), fmt.Sprintf("gcs-core-%d", os.Getuid()))
	} else {
		dir = filepath.Join(dir, "gcs-core")
	}
	return filepath.Join(dir, "core.sock")
}

// FindServerBinary locates gcs-core-server: explicit path, $GCS_CORE_SERVER,
// next to the executable, then $PATH. An embedded-binary strategy would
// extract to a cache dir here (see EVALUATION for why that is fragile).
func FindServerBinary(explicit string) (string, error) {
	cands := []string{explicit, os.Getenv("GCS_CORE_SERVER")}
	if exe, err := os.Executable(); err == nil {
		cands = append(cands, filepath.Join(filepath.Dir(exe), "gcs-core-server"))
	}
	for _, c := range cands {
		if c == "" {
			continue
		}
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, nil
		}
	}
	if p, err := exec.LookPath("gcs-core-server"); err == nil {
		return p, nil
	}
	return "", errors.New("gcs-core-server binary not found (set GCS_CORE_SERVER)")
}

func dialable(sock string) bool {
	addr := sock
	if strings.HasPrefix(sock, "@") {
		addr = "\x00" + sock[1:]
	}
	c, err := net.DialTimeout("unix", addr, 200*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// Start returns a Sidecar bound to opts.Socket, spawning the server if needed.
func Start(ctx context.Context, opts Options) (*Sidecar, error) {
	sock := opts.Socket
	if sock == "" {
		sock = defaultSocket()
	}
	s := &Sidecar{Socket: sock}
	spawnMu.Lock()
	defer spawnMu.Unlock()

	if dialable(sock) {
		return s, nil // reuse a running sidecar
	}
	if opts.NoSpawn {
		return nil, fmt.Errorf("no sidecar listening on %s", sock)
	}
	var bin string
	var err error
	if opts.UseEmbedded {
		bin, err = embeddedServer()
	} else {
		bin, err = FindServerBinary(opts.ServerBinary)
		if err != nil { // fall back to the embedded copy if present
			if b2, e2 := embeddedServer(); e2 == nil {
				bin, err = b2, nil
			}
		}
	}
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(sock, "@") {
		if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
			return nil, err
		}
	}
	idle := opts.IdleSecs
	if idle == 0 {
		idle = 60
	}
	args := []string{"--socket", sock, "--idle-secs", strconv.Itoa(idle)}
	if opts.Transport == GRPC {
		args = append(args, "--default-transport", "grpc")
	}
	if opts.ChunkSize > 0 {
		args = append(args, "--chunk-size", strconv.Itoa(int(opts.ChunkSize)))
	}
	if opts.TCP != "" {
		args = append(args, "--tcp", opts.TCP)
	}
	cmd := exec.Command(bin, args...)
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = sysProcAttr() // own process group: don't die with our terminal; PDEATHSIG handles parent exit
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	t0 := time.Now()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("spawn %s: %w", bin, err)
	}
	s.cmd, s.owned, s.PID = cmd, true, cmd.Process.Pid

	ready := make(chan error, 1)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			if strings.HasPrefix(sc.Text(), "READY") {
				ready <- nil
				// keep draining so the child never blocks on stdout
				for sc.Scan() {
				}
				return
			}
		}
		ready <- fmt.Errorf("server exited before READY")
	}()
	select {
	case err := <-ready:
		if err != nil {
			return nil, err
		}
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		return nil, errors.New("timeout waiting for sidecar READY")
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		return nil, ctx.Err()
	}
	// READY is printed before serve(); make sure accept() is live.
	for i := 0; i < 50 && !dialable(sock); i++ {
		time.Sleep(10 * time.Millisecond)
	}
	SpawnLatency = time.Since(t0)
	go func() { _ = cmd.Wait() }() // reap
	return s, nil
}

// SpawnLatency is the time from exec to a dialable socket for the last spawn.
var SpawnLatency time.Duration

// Owned reports whether this process spawned the sidecar.
func (s *Sidecar) Owned() bool { return s.owned }

// Kill terminates a sidecar this process spawned (SIGTERM, then SIGKILL).
func (s *Sidecar) Kill() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.owned || s.cmd == nil || s.cmd.Process == nil {
		return
	}
	_ = terminate(s.cmd.Process)
	done := make(chan struct{})
	go func() { _, _ = s.cmd.Process.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		_ = s.cmd.Process.Kill()
	}
}
