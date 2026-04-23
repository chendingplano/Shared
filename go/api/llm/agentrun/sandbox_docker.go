package agentrun

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DockerSandbox runs a single agent container: streams stdout/stderr as
// Events, kills the container on ctx cancel (via cidfile + `docker kill`),
// and waits for completion. Designed for Runner implementations to embed.
type DockerSandbox struct {
	Image       string            // e.g. "chenweb/agentrun-claude:v1"
	WorkdirHost string            // absolute host path, mounted at /workspace
	Network     string            // "none" (default) or "bridge"
	Env         map[string]string // injected as --env key=value
	MemoryMB    int               // 0 = unlimited
	CPUs        string            // "" = unlimited, e.g. "2" or "1.5"
	CmdArgs     []string          // appended after image name
}

// DockerAvailable calls `docker version` and returns nil iff it succeeds.
// Intended to be called once at server start so we fail fast.
func DockerAvailable(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "docker", "version", "--format", "{{.Client.Version}}")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker not available: %w", err)
	}
	return nil
}

// Run launches the container, streams stdout/stderr into out, and waits.
// Exit code can be retrieved from the returned error via exec.ExitError.
//
// On ctx cancel, the container is killed via `docker kill` using the cidfile.
// out is NOT closed by Run — the caller owns channel lifecycle.
func (s *DockerSandbox) Run(ctx context.Context, out chan<- Event) error {
	if s.Image == "" {
		return fmt.Errorf("DockerSandbox: Image is required")
	}
	if s.WorkdirHost == "" || !filepath.IsAbs(s.WorkdirHost) {
		return fmt.Errorf("DockerSandbox: WorkdirHost must be an absolute path")
	}

	// cidfile holds the container id so we can `docker kill` on cancel.
	cidFile, err := os.CreateTemp("", "agentrun-*.cid")
	if err != nil {
		return fmt.Errorf("cidfile: %w", err)
	}
	cidPath := cidFile.Name()
	_ = cidFile.Close()
	// docker run requires the cidfile NOT exist at invocation time.
	_ = os.Remove(cidPath)
	defer os.Remove(cidPath)

	args := []string{"run", "--rm", "--init", "--cidfile", cidPath}
	network := s.Network
	if network == "" {
		network = "none"
	}
	args = append(args, "--network", network)
	if s.MemoryMB > 0 {
		args = append(args, "--memory", fmt.Sprintf("%dm", s.MemoryMB))
	}
	if s.CPUs != "" {
		args = append(args, "--cpus", s.CPUs)
	}
	args = append(args, "-v", fmt.Sprintf("%s:/workspace:rw", s.WorkdirHost))
	for k, v := range s.Env {
		args = append(args, "--env", k+"="+v)
	}
	args = append(args, s.Image)
	args = append(args, s.CmdArgs...)

	// We deliberately pass a plain exec.Command (not CommandContext) so we
	// can kill the container cleanly via `docker kill` instead of SIGKILL'ing
	// the docker CLI — SIGKILL can leave the container orphaned.
	cmd := exec.Command("docker", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("docker run start: %w", err)
	}

	// Watch the context: on cancel, kill the container via cidfile.
	killerDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			killByCIDFile(cidPath)
		case <-killerDone:
		}
	}()
	defer close(killerDone)

	// Stream stdout + stderr until EOF. The child closes them on exit.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); streamLines(stdout, EventStdout, out) }()
	go func() { defer wg.Done(); streamLines(stderr, EventStderr, out) }()
	wg.Wait()

	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return waitErr
}

// killByCIDFile reads the cidfile (if written) and issues `docker kill`.
// Silent on error — at worst the container times out on its own.
func killByCIDFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	cid := strings.TrimSpace(string(data))
	if cid == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, "docker", "kill", cid).Run()
}

// streamLines reads until EOF, emitting each line as an Event. Long lines
// (>1MB) are dropped by the scanner; binary output should not be piped
// through here.
func streamLines(r io.Reader, kind EventKind, out chan<- Event) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		out <- Event{Kind: kind, Payload: scanner.Text(), At: time.Now()}
	}
}
