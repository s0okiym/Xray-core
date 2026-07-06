package xproto

import (
	"testing"

	"github.com/xtls/xray-core/transport/internet/reality"
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
	// 200 draws over 3 entries: seeing fewer than 2 distinct is astronomically
	// unlikely, so this guards against pickDest always returning the same entry.
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

func contains(pool []string, v string) bool {
	for _, x := range pool {
		if x == v {
			return true
		}
	}
	return false
}
