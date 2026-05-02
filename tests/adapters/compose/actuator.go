package compose

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
)

type Actuator struct {
	projectName string
	projectDir  string
	profile     string
	log         *slog.Logger
}

func NewActuator(projectName, projectDir, profile string, log *slog.Logger) *Actuator {
	return &Actuator{
		projectName: projectName,
		projectDir:  projectDir,
		profile:     profile,
		log:         log.With("adapter", "docker-compose"),
	}
}

func (a *Actuator) ComposeUp(ctx context.Context, wait bool, services ...string) error {
	a.log.Debug("compose.up", "wait", wait, "services", services)

	args := []string{"up", "--detach"}
	if wait {
		args = append(args, "--wait")
	}
	args = append(args, services...)

	if err := a.runCompose(ctx, args...); err != nil {
		return fmt.Errorf("compose up failed: %w", err)
	}
	return nil
}

func (a *Actuator) ComposeStop(ctx context.Context, timeoutSeconds int, services ...string) error {
	a.log.Debug("compose.stop", "timeout", timeoutSeconds, "services", services)

	args := []string{"stop", "--timeout", strconv.Itoa(timeoutSeconds)}
	args = append(args, services...)

	if err := a.runCompose(ctx, args...); err != nil {
		return fmt.Errorf("compose stop failed: %w", err)
	}
	return nil
}

func (a *Actuator) ComposeDown(ctx context.Context, services ...string) error {
	a.log.Debug("compose.rm", "services", services)

	args := []string{"rm", "--force", "--stop"}
	args = append(args, services...)

	if err := a.runCompose(ctx, args...); err != nil {
		return fmt.Errorf("compose rm failed: %w", err)
	}
	return nil
}

func (a *Actuator) RemoveVolumes(ctx context.Context, volumes ...string) error {
	if len(volumes) == 0 {
		return nil
	}
	a.log.Debug("docker.volume.rm", "volumes", volumes)

	args := append([]string{"volume", "rm", "--force"}, volumes...)

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker volume rm failed: %w", err)
	}
	return nil
}

func (a *Actuator) runCompose(ctx context.Context, commandArgs ...string) error {
	baseArgs := []string{
		"compose",
		"--project-name", a.projectName,
		"--project-directory", a.projectDir,
	}

	if a.profile != "" {
		baseArgs = append(baseArgs, "--profile", a.profile)
	}

	fullArgs := append(baseArgs, commandArgs...)

	cmd := exec.CommandContext(ctx, "docker", fullArgs...)

	cmd.Env = os.Environ()

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
