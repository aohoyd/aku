package portforwards

import (
	"testing"

	"github.com/aohoyd/aku/internal/portforward"
)

func TestPluginColumns(t *testing.T) {
	r := portforward.NewRegistry()
	p := New(r)
	cols := p.Columns()
	if len(cols) != 4 {
		t.Fatalf("expected 4 columns, got %d", len(cols))
	}
	expected := []string{"POD", "CONTAINER", "PORTS", "STATUS"}
	for i, col := range cols {
		if col.Title != expected[i] {
			t.Errorf("column %d: expected %s, got %s", i, expected[i], col.Title)
		}
	}
}

func TestPluginObjects(t *testing.T) {
	r := portforward.NewRegistry()
	r.Add(portforward.Entry{
		PodName:       "nginx-abc",
		ContainerName: "nginx",
		LocalPort:     8080,
		RemotePort:    80,
		Protocol:      "TCP",
		Status:        "Active",
	})
	p := New(r)
	objs := p.Objects()
	if len(objs) != 1 {
		t.Fatalf("expected 1 object, got %d", len(objs))
	}
	row := p.Row(objs[0])
	if row[0] != "nginx-abc" {
		t.Errorf("expected pod name nginx-abc, got %s", row[0])
	}
	if row[2] != "8080:80/TCP" {
		t.Errorf("expected ports 8080:80/TCP, got %s", row[2])
	}
}

func TestPluginName(t *testing.T) {
	r := portforward.NewRegistry()
	p := New(r)
	if p.Name() != "portforwards" {
		t.Errorf("expected name portforwards, got %s", p.Name())
	}
	if p.ShortName() != "pf" {
		t.Errorf("expected short name pf, got %s", p.ShortName())
	}
}

func TestPluginEmptyRegistry(t *testing.T) {
	r := portforward.NewRegistry()
	p := New(r)
	objs := p.Objects()
	if len(objs) != 0 {
		t.Fatalf("expected 0 objects, got %d", len(objs))
	}
}
