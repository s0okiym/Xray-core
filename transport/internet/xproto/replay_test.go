package xproto

import (
	"bufio"
	"bytes"
	"sync"
	"testing"
	"time"
)

func TestReplayGuardAllowNew(t *testing.T) {
	g := newReplayGuard(time.Minute)
	var h [32]byte
	h[0] = 1
	if !g.allow(h) {
		t.Fatal("first allow should return true")
	}
}

func TestReplayGuardRejectReplay(t *testing.T) {
	g := newReplayGuard(time.Minute)
	var h [32]byte
	h[0] = 2
	if !g.allow(h) {
		t.Fatal("first allow should return true")
	}
	if g.allow(h) {
		t.Fatal("second allow should return false (replay)")
	}
}

func TestReplayGuardTtlExpiry(t *testing.T) {
	g := newReplayGuard(40 * time.Millisecond)
	var h [32]byte
	h[0] = 3
	if !g.allow(h) {
		t.Fatal("first allow should return true")
	}
	time.Sleep(80 * time.Millisecond)
	if !g.allow(h) {
		t.Fatal("allow after ttl expiry should return true")
	}
}

func TestReplayGuardConcurrent(t *testing.T) {
	g := newReplayGuard(time.Minute)
	var h [32]byte
	h[0] = 4
	var allowed int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if g.allow(h) {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if allowed != 1 {
		t.Fatalf("expected exactly 1 allow among 100 concurrent, got %d", allowed)
	}
}

func TestClientHelloHashValid(t *testing.T) {
	payload := []byte{0x01, 0x00, 0x00, 0x05, 0x00, 0x00, 0x00, 0x00} // handshake type 0x01 + ...
	rec := bytes.NewBuffer(nil)
	rec.WriteByte(0x16) // TLS handshake
	rec.WriteByte(0x03)
	rec.WriteByte(0x03)
	rec.WriteByte(byte(len(payload) >> 8))
	rec.WriteByte(byte(len(payload)))
	rec.Write(payload)
	br := bufio.NewReader(rec)
	if _, ok := clientHelloHash(br); !ok {
		t.Fatal("expected ok for valid TLS record")
	}
}

func TestClientHelloHashNonTLS(t *testing.T) {
	br := bufio.NewReader(bytes.NewReader([]byte{0x05, 0x01, 0x00, 0x01, 0x00})) // SOCKS5
	if _, ok := clientHelloHash(br); ok {
		t.Fatal("expected not ok for non-TLS (SOCKS5)")
	}
}

func TestClientHelloHashTruncated(t *testing.T) {
	// claims 32-byte payload but only 1 byte follows
	br := bufio.NewReader(bytes.NewReader([]byte{0x16, 0x03, 0x03, 0x00, 0x20, 0x01}))
	if _, ok := clientHelloHash(br); ok {
		t.Fatal("expected not ok for truncated record")
	}
}
