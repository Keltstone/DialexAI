// Package sandbox runs untrusted work (external engines, model-written tests) in a throwaway container:
// no network, no capabilities, resource limits, never the Docker socket.
package sandbox

import (
	"context"
	"os/exec"
	"strconv"
)

type Docker struct {
	Image   string
	CPUs    float64 // default 1
	MemoryM int     // default 1024
	Bin     string  // "docker" or "podman"; podman rootless is preferred
}

func (d Docker) base(name string) []string {
	cpus, mem, bin := d.CPUs, d.MemoryM, d.Bin
	if cpus == 0 {
		cpus = 1
	}
	if mem == 0 {
		mem = 1024
	}
	if bin == "" {
		bin = "docker"
	}
	return []string{bin, "run", "--rm", "--network", "none", "--cap-drop", "ALL", "--security-opt", "no-new-privileges",
		"--pids-limit", "512", "--cpus", strconv.FormatFloat(cpus, 'f', -1, 64), "--memory", strconv.Itoa(mem) + "m",
		"--user", "65534:65534", "-w", name}
}

// ScanCmd mounts root read-only at /src and runs bin there. Pass it as engines.Sandbox.
func (d Docker) ScanCmd(root, bin string, args []string) *exec.Cmd {
	a := append(d.base("/src"), "--read-only", "--tmpfs", "/tmp", "-v", root+":/src:ro", d.Image, bin)
	a = append(a, args...)
	a = append(a, "/src")
	return exec.Command(a[0], a[1:]...)
}

// Run executes cmd in a copy of a tree mounted read-write at /work (the copy is thrown away by the caller).
// Its signature matches ai.Runner.
func (d Docker) Run(ctx context.Context, dir string, cmd []string) error {
	a := append(d.base("/work"), "--tmpfs", "/tmp", "-v", dir+":/work", d.Image)
	a = append(a, cmd...)
	return exec.CommandContext(ctx, a[0], a[1:]...).Run()
}
