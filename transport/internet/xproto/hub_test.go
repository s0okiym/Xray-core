package xproto

import (
	"testing"

	"github.com/xtls/xray-core/features"
	"github.com/xtls/xray-core/features/extension"
	"github.com/xtls/xray-core/transport/internet/reality"
)

// mockDynConfig implements extension.DynConfig for testing.
type mockDynConfig struct {
	pool []string
}

func (m *mockDynConfig) Type() interface{}      { return extension.DynConfigType() }
func (m *mockDynConfig) Start() error           { return nil }
func (m *mockDynConfig) Close() error           { return nil }
func (m *mockDynConfig) DestPool() []string     { return m.pool }
func (m *mockDynConfig) SetDestPool(p []string) { m.pool = p }

var (
	_ extension.DynConfig = (*mockDynConfig)(nil)
	_ features.Feature    = (*mockDynConfig)(nil)
)

func TestPickDestUsesPool(t *testing.T) {
	dests := []string{"a.example:443", "b.example:443", "c.example:443"}
	l := &Listener{config: &Config{Dests: dests}}
	seen := make(map[string]bool)
	for i := 0; i < 200; i++ {
		d := l.pickDest()
		if !contains(dests, d) {
			t.Fatalf("pickDest returned %q not in pool %v", d, dests)
		}
		seen[d] = true
	}
	if len(seen) < 2 {
		t.Fatalf("pickDest not randomizing: only saw %v after 200 draws", seen)
	}
}

func TestPickDestFallbackToBase(t *testing.T) {
	l := &Listener{config: &Config{
		Base: &reality.Config{Dest: "fixed.example:443"},
	}}
	for i := 0; i < 20; i++ {
		if d := l.pickDest(); d != "fixed.example:443" {
			t.Fatalf("pickDest = %q, want fixed.example:443", d)
		}
	}
}

func TestPickDestEmptyPoolAndBase(t *testing.T) {
	l := &Listener{config: &Config{Base: &reality.Config{}}}
	if d := l.pickDest(); d != "" {
		t.Fatalf("pickDest = %q, want empty", d)
	}
}

func TestPickDestPrefersDynConfig(t *testing.T) {
	dynPool := []string{"dyn1.example:443", "dyn2.example:443"}
	staticPool := []string{"static1.example:443", "static2.example:443"}
	l := &Listener{
		config: &Config{Dests: staticPool},
		dyn:    &mockDynConfig{pool: dynPool},
	}
	seen := make(map[string]bool)
	for i := 0; i < 200; i++ {
		d := l.pickDest()
		if !contains(dynPool, d) {
			t.Fatalf("pickDest returned %q not in dyn pool %v (should prefer dyn)", d, dynPool)
		}
		seen[d] = true
	}
	if len(seen) < 2 {
		t.Fatalf("pickDest not randomizing over dyn pool: only saw %v", seen)
	}
}

func TestPickDestDynConfigEmptyFallsToStatic(t *testing.T) {
	staticPool := []string{"static1.example:443", "static2.example:443"}
	l := &Listener{
		config: &Config{Dests: staticPool},
		dyn:    &mockDynConfig{pool: nil},
	}
	for i := 0; i < 20; i++ {
		d := l.pickDest()
		if !contains(staticPool, d) {
			t.Fatalf("pickDest = %q, want a static pool entry (dyn empty)", d)
		}
	}
}

func contains(pool []string, v string) bool {
	for _, x := range pool {
		if x == v {
			return true
		}
	}
	return false
}
