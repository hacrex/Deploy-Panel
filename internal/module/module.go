// Package module defines the contract for optional panel modules
// (wordpress, dns, mail, waf, resell, import). See docs/MODULES.md.
package module

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
)

// Host is the controlled surface a module gets from core. Concrete services
// are added as core packages land; keep this interface small.
type Host interface {
	// Module returns another enabled module's public interface, or an error
	// if it is disabled. Modules must not read each other's tables.
	Module(name string) (any, error)
}

// Module is implemented by every optional module under mod/<name>.
type Module interface {
	Name() string
	Version() string
	Requires() []string // names of other modules; core is always available
	Migrations() fs.FS  // embedded SQL, tables prefixed with the module name
	Init(ctx context.Context, h Host) error
	Close() error
}

// Registry holds the modules compiled into this binary.
type Registry struct {
	mods map[string]Module
}

func NewRegistry() *Registry { return &Registry{mods: map[string]Module{}} }

func (r *Registry) Register(m Module) error {
	if _, dup := r.mods[m.Name()]; dup {
		return fmt.Errorf("module %q already registered", m.Name())
	}
	r.mods[m.Name()] = m
	return nil
}

// Order returns the enabled modules sorted so that dependencies come first.
// It fails on a missing dependency, a dependency on a disabled module, or a cycle.
func (r *Registry) Order(enabled []string) ([]Module, error) {
	on := map[string]bool{}
	for _, n := range enabled {
		if _, ok := r.mods[n]; !ok {
			return nil, fmt.Errorf("module %q is not available in this build", n)
		}
		on[n] = true
	}
	names := make([]string, 0, len(on))
	for n := range on {
		names = append(names, n)
	}
	sort.Strings(names)

	var out []Module
	state := map[string]int{} // 0 new, 1 visiting, 2 done
	var visit func(n string) error
	visit = func(n string) error {
		switch state[n] {
		case 1:
			return fmt.Errorf("dependency cycle at module %q", n)
		case 2:
			return nil
		}
		state[n] = 1
		m := r.mods[n]
		deps := append([]string(nil), m.Requires()...)
		sort.Strings(deps)
		for _, d := range deps {
			if !on[d] {
				return fmt.Errorf("module %q requires %q, which is not enabled", n, d)
			}
			if err := visit(d); err != nil {
				return err
			}
		}
		state[n] = 2
		out = append(out, m)
		return nil
	}
	for _, n := range names {
		if err := visit(n); err != nil {
			return nil, err
		}
	}
	return out, nil
}
