package workload

import (
	"strings"
	"testing"

	"github.com/aohoyd/aku/internal/render"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

func TestDescribeConditions(t *testing.T) {
	deploy := &appsv1.Deployment{
		Status: appsv1.DeploymentStatus{
			Conditions: []appsv1.DeploymentCondition{
				{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
				{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue},
			},
		},
	}
	b := render.NewBuilder()
	DescribeConditions(b, deploy)
	content := b.Build()
	if !strings.Contains(content.Raw, "Conditions") {
		t.Fatal("expected Conditions section")
	}
	if !strings.Contains(content.Raw, "Available") {
		t.Fatal("expected Available condition")
	}
}

func TestDescribeConditionsEmpty(t *testing.T) {
	deploy := &appsv1.Deployment{}
	b := render.NewBuilder()
	DescribeConditions(b, deploy)
	content := b.Build()
	if strings.Contains(content.Raw, "Conditions") {
		t.Fatal("expected no Conditions section for empty conditions")
	}
}
