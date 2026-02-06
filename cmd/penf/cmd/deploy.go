package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// serviceConfig defines the build and deploy configuration for a service.
type serviceConfig struct {
	Name       string
	GOOS       string
	GOARCH     string
	BuildDir   string
	BinaryName string
	Host       string
	HostEnvVar string
	BinaryPath string
	NomadJob   string
	NomadName  string
}

var services = map[string]serviceConfig{
	"gateway": {
		Name:       "gateway",
		GOOS:       "linux",
		GOARCH:     "amd64",
		BuildDir:   "services/gateway",
		BinaryName: "gateway-linux",
		Host:       "dev02",
		HostEnvVar: "GATEWAY_HOST",
		BinaryPath: "/opt/penfold/bin/penfold-gateway",
		NomadJob:   "deploy/nomad/gateway.nomad.hcl",
		NomadName:  "penfold-gateway",
	},
	"worker": {
		Name:       "worker",
		GOOS:       "darwin",
		GOARCH:     "arm64",
		BuildDir:   "services/worker",
		BinaryName: "worker-darwin-arm64",
		Host:       "dev01",
		HostEnvVar: "WORKER_HOST",
		BinaryPath: "/opt/penfold/bin/penfold-worker",
		NomadJob:   "deploy/nomad/worker.nomad.hcl",
		NomadName:  "penfold-worker",
	},
	"ai": {
		Name:       "ai-coordinator",
		GOOS:       "linux",
		GOARCH:     "amd64",
		BuildDir:   "services/ai",
		BinaryName: "ai-coordinator-linux",
		Host:       "dev02",
		HostEnvVar: "AI_HOST",
		BinaryPath: "/opt/penfold/bin/penfold-ai-coordinator",
		NomadJob:   "deploy/nomad/ai-coordinator.nomad.hcl",
		NomadName:  "penfold-ai-coordinator",
	},
}

var deployStatus bool

// NewDeployCommand creates the deploy command.
func NewDeployCommand() *cobra.Command {
	deployCmd := &cobra.Command{
		Use:   "deploy [gateway|worker|ai|all]",
		Short: "Build, upload, and deploy services via Nomad",
		Long: `Build, upload, and deploy Penfold services using Nomad.

Each service is cross-compiled, uploaded via SCP, and deployed using
'nomad job run'. Nomad handles health checks, canary promotion, and
auto-revert on failure.

Examples:
  penf deploy gateway      Build and deploy gateway to dev02
  penf deploy worker       Build and deploy worker to dev01
  penf deploy ai           Build and deploy AI coordinator to dev02
  penf deploy all          Deploy all services in order
  penf deploy --status     Show Nomad job status for all services

Environment:
  NOMAD_ADDR       Nomad server address (default: http://dev02.brown.chat:4646)
  GATEWAY_HOST     Gateway host (default: dev02)
  WORKER_HOST      Worker host (default: dev01)
  AI_HOST          AI coordinator host (default: dev02)`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if deployStatus {
				return runDeployStatus()
			}
			if len(args) == 0 {
				return cmd.Help()
			}
			target := args[0]
			if target == "all" {
				return runDeployAll()
			}
			svc, ok := services[target]
			if !ok {
				return fmt.Errorf("unknown service: %s (valid: gateway, worker, ai, all)", target)
			}
			return runDeploy(svc)
		},
	}

	deployCmd.Flags().BoolVar(&deployStatus, "status", false, "Show Nomad job status for all services")

	return deployCmd
}

func nomadAddr() string {
	if addr := os.Getenv("NOMAD_ADDR"); addr != "" {
		return addr
	}
	return "http://dev02.brown.chat:4646"
}

func projectRoot() (string, error) {
	// Walk up from the executable or current directory to find go.mod.
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("cannot find project root (no go.mod found)")
		}
		dir = parent
	}
}

func hostForService(svc serviceConfig) string {
	if h := os.Getenv(svc.HostEnvVar); h != "" {
		return h
	}
	return svc.Host
}

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runCmdEnv(env []string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), env...)
	return cmd.Run()
}

func runDeploy(svc serviceConfig) error {
	root, err := projectRoot()
	if err != nil {
		return err
	}

	host := hostForService(svc)

	fmt.Printf("=== Deploying %s ===\n\n", svc.Name)

	// 1. Build
	fmt.Printf("[1/3] Building %s (%s/%s)...\n", svc.Name, svc.GOOS, svc.GOARCH)
	buildDir := filepath.Join(root, svc.BuildDir)
	buildOutput := filepath.Join(buildDir, svc.BinaryName)

	buildCmd := exec.Command("go", "build", "-o", buildOutput, ".")
	buildCmd.Dir = buildDir
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	buildCmd.Env = append(os.Environ(),
		"GOOS="+svc.GOOS,
		"GOARCH="+svc.GOARCH,
	)
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	fi, err := os.Stat(buildOutput)
	if err != nil {
		return fmt.Errorf("build output not found: %w", err)
	}
	fmt.Printf("  Built %s (%.1f MB)\n\n", svc.BinaryName, float64(fi.Size())/(1024*1024))

	// 2. Upload
	fmt.Printf("[2/3] Uploading to %s:%s...\n", host, svc.BinaryPath)
	if err := runCmd("scp", buildOutput, fmt.Sprintf("%s:%s.new", host, svc.BinaryPath)); err != nil {
		return fmt.Errorf("scp failed: %w", err)
	}
	if err := runCmd("ssh", host, fmt.Sprintf("chmod +x %s.new && mv %s.new %s", svc.BinaryPath, svc.BinaryPath, svc.BinaryPath)); err != nil {
		return fmt.Errorf("binary swap failed: %w", err)
	}
	fmt.Printf("  Uploaded\n\n")

	// 3. Nomad job run
	fmt.Printf("[3/3] Submitting Nomad job: %s...\n", svc.NomadName)
	jobFile := filepath.Join(root, svc.NomadJob)
	if err := runCmdEnv([]string{"NOMAD_ADDR=" + nomadAddr()}, "nomad", "job", "run", jobFile); err != nil {
		return fmt.Errorf("nomad job run failed: %w", err)
	}

	// Wait for healthy
	fmt.Printf("  Waiting for %s to be healthy...\n", svc.NomadName)
	if err := waitForNomadHealthy(svc.NomadName, 60); err != nil {
		return err
	}

	fmt.Printf("\n=== %s deployed successfully ===\n", svc.Name)
	return nil
}

func runDeployAll() error {
	// Deploy in dependency order: gateway -> worker -> ai
	order := []string{"gateway", "worker", "ai"}
	for _, name := range order {
		svc := services[name]
		if err := runDeploy(svc); err != nil {
			return fmt.Errorf("deploy %s failed: %w", name, err)
		}
		fmt.Println()
	}
	fmt.Println("=== All services deployed ===")
	return nil
}

func runDeployStatus() error {
	addr := nomadAddr()
	fmt.Printf("Nomad: %s\n\n", addr)
	fmt.Printf("%-25s %s\n", "JOB", "STATUS")
	fmt.Printf("%-25s %s\n", "---", "------")

	for _, name := range []string{"gateway", "worker", "ai"} {
		svc := services[name]
		status := getNomadJobStatus(svc.NomadName)
		fmt.Printf("%-25s %s\n", svc.NomadName, status)
	}
	return nil
}

func getNomadJobStatus(jobName string) string {
	cmd := exec.Command("nomad", "job", "status", "-short", jobName)
	cmd.Env = append(os.Environ(), "NOMAD_ADDR="+nomadAddr())
	out, err := cmd.Output()
	if err != nil {
		return "not found"
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "Status") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				return parts[len(parts)-1]
			}
		}
	}
	return "unknown"
}

func waitForNomadHealthy(jobName string, timeoutSecs int) error {
	for i := 0; i < timeoutSecs; i++ {
		status := getNomadJobStatus(jobName)
		if status == "running" {
			fmt.Printf("  %s is running\n", jobName)
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("%s failed to become healthy within %ds", jobName, timeoutSecs)
}
