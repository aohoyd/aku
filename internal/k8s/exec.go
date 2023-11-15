package k8s

import (
	"os/exec"

	"github.com/aohoyd/aku/internal/msgs"
	tea "charm.land/bubbletea/v2"
)

// ExecCmd returns a tea.Cmd that suspends the TUI and runs kubectl exec.
func ExecCmd(podName, containerName, namespace string) tea.Cmd {
	args := []string{"exec", "-it", "-n", namespace, podName}
	if containerName != "" {
		args = append(args, "-c", containerName)
	}
	args = append(args, "--", "/bin/sh")

	c := exec.Command("kubectl", args...)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		if err != nil {
			return msgs.ActionResultMsg{Err: err}
		}
		return msgs.ActionResultMsg{ActionID: "exec:" + podName}
	})
}
