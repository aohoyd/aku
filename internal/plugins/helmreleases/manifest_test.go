package helmreleases

import (
	"testing"
)

func TestParseManifestMultiDoc(t *testing.T) {
	manifest := "---\napiVersion: v1\nkind: Service\nmetadata:\n  name: my-svc\n  namespace: default\n---\napiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: my-deploy\n  namespace: default\nspec:\n  replicas: 1\n"
	objs := ParseManifest(manifest)
	if len(objs) != 2 {
		t.Fatalf("expected 2 objects, got %d", len(objs))
	}

	if objs[0].GetKind() != "Service" {
		t.Fatalf("expected Service, got %s", objs[0].GetKind())
	}
	if objs[0].GetName() != "my-svc" {
		t.Fatalf("expected my-svc, got %s", objs[0].GetName())
	}

	if objs[1].GetKind() != "Deployment" {
		t.Fatalf("expected Deployment, got %s", objs[1].GetKind())
	}
}

func TestParseManifestEmptySeparators(t *testing.T) {
	manifest := "---\n---\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm\n---\n"
	objs := ParseManifest(manifest)
	if len(objs) != 1 {
		t.Fatalf("expected 1 object, got %d", len(objs))
	}
}

func TestParseManifestEmpty(t *testing.T) {
	objs := ParseManifest("")
	if len(objs) != 0 {
		t.Fatalf("expected 0 objects, got %d", len(objs))
	}
}

func TestParseManifestMalformed(t *testing.T) {
	manifest := "---\nnot: valid: yaml: {{{\n---\napiVersion: v1\nkind: Secret\nmetadata:\n  name: s\n"
	objs := ParseManifest(manifest)
	if len(objs) != 1 {
		t.Fatalf("expected 1 object (malformed skipped), got %d", len(objs))
	}
}
