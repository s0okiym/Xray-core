package dynconfig

import "testing"

func TestDestPoolSetAndGet(t *testing.T) {
	d := &DynConfig{}
	if pool := d.DestPool(); len(pool) != 0 {
		t.Fatalf("initial pool = %v, want empty", pool)
	}
	d.SetDestPool([]string{"a:443", "b:443"})
	got := d.DestPool()
	if len(got) != 2 || got[0] != "a:443" || got[1] != "b:443" {
		t.Fatalf("DestPool = %v, want [a:443 b:443]", got)
	}
}

func TestDestPoolIsolation(t *testing.T) {
	d := &DynConfig{}
	original := []string{"a:443", "b:443"}
	d.SetDestPool(original)
	original[0] = "MUTATED"
	if got := d.DestPool(); got[0] != "a:443" {
		t.Fatalf("SetDestPool did not copy input: %v", got)
	}
	got := d.DestPool()
	got[0] = "MUTATED"
	if got2 := d.DestPool(); got2[0] != "a:443" {
		t.Fatalf("DestPool did not return a copy: %v", got2)
	}
}

func TestDestPoolConcurrent(t *testing.T) {
	d := &DynConfig{destPool: []string{"a:443"}}
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			d.SetDestPool([]string{"x:443"})
		}
		close(done)
	}()
	for i := 0; i < 100; i++ {
		_ = d.DestPool()
	}
	<-done
}
