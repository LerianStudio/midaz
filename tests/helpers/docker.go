package helpers

import (
    "fmt"
    "os/exec"
    "strings"
    "time"
)

// ComposeUpBackend brings infra + onboarding + transaction online using root Makefile.
func ComposeUpBackend() error {
    cmd := exec.Command("make", "up-backend")
    out, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("make up-backend failed: %v\n%s", err, string(out))
    }
    return nil
}

// ComposeDownBackend stops services started with ComposeUpBackend.
func ComposeDownBackend() error {
    cmd := exec.Command("make", "down-backend")
    out, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("make down-backend failed: %v\n%s", err, string(out))
    }
    return nil
}

// DockerAction performs a docker container action like stop/start/restart/pause/unpause.
func DockerAction(action, container string, extraArgs ...string) error {
    args := append([]string{action, container}, extraArgs...)
    cmd := exec.Command("docker", args...)
    out, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("docker %s %s failed: %v\n%s", action, strings.Join(args, " "), err, string(out))
    }
    return nil
}

// RestartWithWait restarts a container and waits a bit for it to come back.
func RestartWithWait(container string, wait time.Duration) error {
    if err := DockerAction("restart", container); err != nil {
        return err
    }
    time.Sleep(wait)
    return nil
}

// DockerNetwork connects or disconnects a container to/from a Docker network.
// action should be "connect" or "disconnect".
func DockerNetwork(action, network, container string) error {
    cmd := exec.Command("docker", "network", action, network, container)
    out, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("docker network %s %s %s failed: %v\n%s", action, network, container, err, string(out))
    }
    return nil
}

// DockerExec runs a command inside a running container and returns its combined output.
func DockerExec(container string, args ...string) (string, error) {
    full := append([]string{"exec", container}, args...)
    cmd := exec.Command("docker", full...)
    out, err := cmd.CombinedOutput()
    if err != nil {
        return string(out), fmt.Errorf("docker exec %s %v failed: %v\n%s", container, args, err, string(out))
    }
    return string(out), nil
}
