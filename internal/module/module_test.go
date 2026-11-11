package module

import (
	"context"
	"io/fs"
	"testing"
)

type fake struct {
	name string
	deps []string
}

func (f fake) Name() string                     { return f.name }
func (f fake) Version() string                  { return "0.0.0" }
func (f fake) Requires() []string               { return f.deps }
func (f fake) Migrations() fs.FS                { return nil }
func (f fake) Init(context.Context, Host) error { return nil }
func (f fake) Close() error                     { return nil }

func TestOrder(t *testing.T) {
	r := NewRegistry()
	for _, m := range []fake{{"dns", nil}, {"mail", []string{"dns"}}, {"waf", nil}} {
		if err := r.Register(m); err != nil {
			t.Fatal(err)
		}
	}

	got, err := r.Order([]string{"mail", "dns"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name() != "dns" || got[1].Name() != "mail" {
		t.Fatalf("wrong order: %v", got)
	}

	if _, err := r.Order([]string{"mail"}); err == nil {
		t.Fatal("expected error when dependency is not enabled")
	}
	if _, err := r.Order([]string{"nope"}); err == nil {
		t.Fatal("expected error for unknown module")
	}
}

func TestCycle(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(fake{"a", []string{"b"}})
	_ = r.Register(fake{"b", []string{"a"}})
	if _, err := r.Order([]string{"a", "b"}); err == nil {
		t.Fatal("expected cycle error")
	}
}
