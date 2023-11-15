package containers

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/aohoyd/aku/internal/render"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// DescribeContainer writes a complete container description into b at the given
// indentation level: spec, status, resources, probes, env, mounts.
//
// status may be nil (e.g. deployment templates with no runtime status).
// pod, configMaps, and secrets may be nil (resolution disabled).
func DescribeContainer(
	b *render.Builder,
	level int,
	c corev1.Container,
	status *corev1.ContainerStatus,
	pod *corev1.Pod,
	configMaps []*unstructured.Unstructured,
	secrets []*unstructured.Unstructured,
) {
	var cms, secs map[string]*unstructured.Unstructured
	if len(configMaps) > 0 || len(secrets) > 0 {
		cms = indexByName(configMaps)
		secs = indexByName(secrets)
	}

	describeSpec(b, level, c)
	if status != nil {
		describeContainerStatus(b, level, *status)
	}
	describeResources(b, level, c)
	describeProbes(b, level, c)
	describeEnvFrom(b, level, c, cms, secs)
	describeEnv(b, level, c, pod, cms, secs)
	describeMounts(b, level, c)
}

// --- container section helpers ---

func describeSpec(b *render.Builder, level int, c corev1.Container) {
	b.KV(level, "Image", c.Image)

	if len(c.Ports) > 0 {
		var portStrs []string
		var hostPortStrs []string
		for _, p := range c.Ports {
			proto := string(p.Protocol)
			if proto == "" {
				proto = "TCP"
			}
			s := fmt.Sprintf("%d/%s", p.ContainerPort, proto)
			if p.Name != "" {
				s += " (" + p.Name + ")"
			}
			portStrs = append(portStrs, s)

			if p.HostPort > 0 {
				hs := fmt.Sprintf("%d/%s", p.HostPort, proto)
				if p.Name != "" {
					hs += " (" + p.Name + ")"
				}
				hostPortStrs = append(hostPortStrs, hs)
			}
		}
		key := "Port"
		if len(portStrs) > 1 {
			key = "Ports"
		}
		b.KV(level, key, strings.Join(portStrs, ", "))
		if len(hostPortStrs) > 0 {
			hKey := "Host Port"
			if len(hostPortStrs) > 1 {
				hKey = "Host Ports"
			}
			b.KV(level, hKey, strings.Join(hostPortStrs, ", "))
		}
	}
}

func describeResources(b *render.Builder, level int, c corev1.Container) {
	if limits := resourceListToMap(c.Resources.Limits); len(limits) > 0 {
		b.KV(level, "Limits", formatResources(limits))
	}
	if requests := resourceListToMap(c.Resources.Requests); len(requests) > 0 {
		b.KV(level, "Requests", formatResources(requests))
	}
}

func describeProbes(b *render.Builder, level int, c corev1.Container) {
	if probe := formatProbe(c.LivenessProbe); probe != "" {
		b.KV(level, "Liveness", probe)
	}
	if probe := formatProbe(c.ReadinessProbe); probe != "" {
		b.KV(level, "Readiness", probe)
	}
	if probe := formatProbe(c.StartupProbe); probe != "" {
		b.KV(level, "Startup", probe)
	}
}

func describeMounts(b *render.Builder, level int, c corev1.Container) {
	if len(c.VolumeMounts) > 0 {
		b.Section(level, "Mounts")
		for _, m := range c.VolumeMounts {
			ro := " (rw)"
			if m.ReadOnly {
				ro = " (ro)"
			}
			b.KV(level+1, m.MountPath, "from "+m.Name+ro, render.Unaligned())
		}
	} else {
		b.KV(level, "Mounts", "<none>")
	}
}

func describeEnv(b *render.Builder, level int, c corev1.Container, pod *corev1.Pod, cms, secrets map[string]*unstructured.Unstructured) {
	if len(c.Env) > 0 {
		b.Section(level, "Environment")
		for _, e := range c.Env {
			if e.Value != "" {
				b.KV(level+1, e.Name, e.Value)
			} else if e.ValueFrom != nil {
				if resolved := resolveEnv(e, pod, &c, cms, secrets); resolved != "" {
					b.KV(level+1, e.Name, resolved)
					continue
				}
				b.KV(level+1, e.Name, formatValueFrom(e.ValueFrom))
			} else {
				b.Section(level+1, e.Name)
			}
		}
	} else if len(c.EnvFrom) == 0 {
		b.KV(level, "Environment", "<none>")
	}
}

func describeEnvFrom(b *render.Builder, level int, c corev1.Container, cms, secrets map[string]*unstructured.Unstructured) {
	if len(c.EnvFrom) > 0 {
		b.Section(level, "Environment Variables from")
		for _, ef := range c.EnvFrom {
			if ef.ConfigMapRef != nil {
				name := ef.ConfigMapRef.Name
				optional := ef.ConfigMapRef.Optional != nil && *ef.ConfigMapRef.Optional
				b.KV(level+1, name, fmt.Sprintf("ConfigMap  Optional: %v", optional))
				if cms != nil || secrets != nil {
					if vals := resolveEnvFrom(ef, cms, secrets); vals != nil {
						writeEnvFromValues(b, level+2, vals)
					}
				}
			}
			if ef.SecretRef != nil {
				name := ef.SecretRef.Name
				optional := ef.SecretRef.Optional != nil && *ef.SecretRef.Optional
				b.KV(level+1, name, fmt.Sprintf("Secret  Optional: %v", optional))
				if cms != nil || secrets != nil {
					if vals := resolveEnvFrom(ef, cms, secrets); vals != nil {
						writeEnvFromValues(b, level+2, vals)
					}
				}
			}
		}
	}
}

// --- container status helpers ---

func describeContainerStatus(b *render.Builder, level int, cs corev1.ContainerStatus) {
	switch {
	case cs.State.Running != nil:
		b.KVStyled(level, render.ValueStatusOK, "State", "Running")
		if !cs.State.Running.StartedAt.IsZero() {
			b.KV(level+1, "Started", cs.State.Running.StartedAt.Format(time.RFC1123Z))
		}
	case cs.State.Waiting != nil:
		reason := cs.State.Waiting.Reason
		if reason == "" {
			reason = "Waiting"
		}
		b.KVStyled(level, render.StatusKind(reason), "State", "Waiting")
		b.KV(level+1, "Reason", reason)
		if cs.State.Waiting.Message != "" {
			b.KV(level+1, "Message", cs.State.Waiting.Message)
		}
	case cs.State.Terminated != nil:
		reason := cs.State.Terminated.Reason
		if reason == "" {
			reason = "Completed"
		}
		b.KVStyled(level, render.StatusKind(reason), "State", "Terminated")
		b.KV(level+1, "Reason", reason)
		if cs.State.Terminated.ExitCode != 0 {
			b.KV(level+1, "Exit Code", fmt.Sprintf("%d", cs.State.Terminated.ExitCode))
		}
		if !cs.State.Terminated.StartedAt.IsZero() {
			b.KV(level+1, "Started", cs.State.Terminated.StartedAt.Format(time.RFC1123Z))
		}
		if !cs.State.Terminated.FinishedAt.IsZero() {
			b.KV(level+1, "Finished", cs.State.Terminated.FinishedAt.Format(time.RFC1123Z))
		}
	}

	describeLastState(b, level, cs.LastTerminationState)

	b.KV(level, "Ready", fmt.Sprintf("%v", cs.Ready))
	b.KV(level, "Restart Count", fmt.Sprintf("%d", cs.RestartCount))
}

func describeLastState(b *render.Builder, level int, state corev1.ContainerState) {
	switch {
	case state.Terminated != nil:
		reason := state.Terminated.Reason
		if reason == "" {
			reason = "Completed"
		}
		b.KV(level, "Last State", "Terminated")
		b.KV(level+1, "Reason", reason)
		if state.Terminated.ExitCode != 0 {
			b.KV(level+1, "Exit Code", fmt.Sprintf("%d", state.Terminated.ExitCode))
		}
		if !state.Terminated.StartedAt.IsZero() {
			b.KV(level+1, "Started", state.Terminated.StartedAt.Format(time.RFC1123Z))
		}
		if !state.Terminated.FinishedAt.IsZero() {
			b.KV(level+1, "Finished", state.Terminated.FinishedAt.Format(time.RFC1123Z))
		}
	case state.Waiting != nil:
		b.KV(level, "Last State", "Waiting")
		if state.Waiting.Reason != "" {
			b.KV(level+1, "Reason", state.Waiting.Reason)
		}
	case state.Running != nil:
		b.KV(level, "Last State", "Running")
		if !state.Running.StartedAt.IsZero() {
			b.KV(level+1, "Started", state.Running.StartedAt.Format(time.RFC1123Z))
		}
	}
}

// --- formatting helpers ---

func writeEnvFromValues(b *render.Builder, level int, vals map[string]string) {
	keys := make([]string, 0, len(vals))
	for k := range vals {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		b.KV(level, k, vals[k])
	}
}

func resourceListToMap(rl corev1.ResourceList) map[string]string {
	m := make(map[string]string, len(rl))
	for k, v := range rl {
		m[string(k)] = v.String()
	}
	return m
}

func formatResources(res map[string]string) string {
	var parts []string
	order := []string{"cpu", "memory"}
	seen := make(map[string]bool)
	for _, k := range order {
		if v, ok := res[k]; ok {
			parts = append(parts, k+": "+v)
			seen[k] = true
		}
	}
	var others []string
	for k := range res {
		if !seen[k] {
			others = append(others, k)
		}
	}
	slices.Sort(others)
	for _, k := range others {
		parts = append(parts, k+": "+res[k])
	}
	return strings.Join(parts, ", ")
}

func formatProbe(p *corev1.Probe) string {
	if p == nil {
		return ""
	}

	var action string
	switch {
	case p.ProbeHandler.HTTPGet != nil:
		httpGet := p.ProbeHandler.HTTPGet
		path := httpGet.Path
		port := httpGet.Port.String()
		scheme := string(httpGet.Scheme)
		if scheme == "" {
			scheme = "HTTP"
		}
		action = fmt.Sprintf("http-get %s://:%s%s", strings.ToLower(scheme), port, path)
	case p.ProbeHandler.TCPSocket != nil:
		port := p.ProbeHandler.TCPSocket.Port.String()
		action = "tcp-socket :" + port
	case p.ProbeHandler.Exec != nil:
		if cmds := p.ProbeHandler.Exec.Command; len(cmds) > 0 {
			action = "exec [" + strings.Join(cmds, " ") + "]"
		}
	case p.ProbeHandler.GRPC != nil:
		port := fmt.Sprintf("%d", p.ProbeHandler.GRPC.Port)
		action = "grpc :" + port
	}

	if action == "" {
		return ""
	}

	var details []string
	if p.InitialDelaySeconds > 0 {
		details = append(details, fmt.Sprintf("delay=%ds", p.InitialDelaySeconds))
	}
	if p.TimeoutSeconds > 0 {
		details = append(details, fmt.Sprintf("timeout=%ds", p.TimeoutSeconds))
	}
	if p.PeriodSeconds > 0 {
		details = append(details, fmt.Sprintf("period=%ds", p.PeriodSeconds))
	}
	if p.SuccessThreshold > 0 {
		details = append(details, fmt.Sprintf("#success=%d", p.SuccessThreshold))
	}
	if p.FailureThreshold > 0 {
		details = append(details, fmt.Sprintf("#failure=%d", p.FailureThreshold))
	}

	if len(details) > 0 {
		return action + " " + strings.Join(details, " ")
	}
	return action
}

func formatValueFrom(vf *corev1.EnvVarSource) string {
	switch {
	case vf.ConfigMapKeyRef != nil:
		name := vf.ConfigMapKeyRef.Name
		key := vf.ConfigMapKeyRef.Key
		optional := vf.ConfigMapKeyRef.Optional != nil && *vf.ConfigMapKeyRef.Optional
		return fmt.Sprintf("<key %s in ConfigMap %s>  Optional: %v", key, name, optional)
	case vf.SecretKeyRef != nil:
		name := vf.SecretKeyRef.Name
		key := vf.SecretKeyRef.Key
		optional := vf.SecretKeyRef.Optional != nil && *vf.SecretKeyRef.Optional
		return fmt.Sprintf("<key %s in Secret %s>  Optional: %v", key, name, optional)
	case vf.FieldRef != nil:
		apiVersion := vf.FieldRef.APIVersion
		if apiVersion == "" {
			apiVersion = "v1"
		}
		return fmt.Sprintf("%s (%s:FieldRef)", vf.FieldRef.FieldPath, apiVersion)
	case vf.ResourceFieldRef != nil:
		containerName := vf.ResourceFieldRef.ContainerName
		return fmt.Sprintf("%s (%s:ResourceFieldRef)", vf.ResourceFieldRef.Resource, containerName)
	}
	return "<set>"
}
