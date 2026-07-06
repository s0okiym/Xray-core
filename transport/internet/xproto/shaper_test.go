package xproto

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
)

func TestHandshakePaddingNil(t *testing.T) {
	c, s := net.Pipe()
	defer c.Close()
	defer s.Close()
	go func() {
		HandshakePadding(c, true, nil)
		c.Write([]byte("X"))
	}()
	if err := HandshakePadding(s, false, nil); err != nil {
		t.Fatalf("server nil scheme: %v", err)
	}
	buf := make([]byte, 1)
	if _, err := io.ReadFull(s, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != "X" {
		t.Fatalf("got %q want X", buf)
	}
}

func TestHandshakePaddingDisabled(t *testing.T) {
	c, s := net.Pipe()
	defer c.Close()
	defer s.Close()
	go func() {
		HandshakePadding(c, true, &PaddingScheme{Enabled: false})
		c.Write([]byte("HI"))
	}()
	if err := HandshakePadding(s, false, &PaddingScheme{Enabled: false}); err != nil {
		t.Fatalf("server: %v", err)
	}
	buf := make([]byte, 2)
	if _, err := io.ReadFull(s, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != "HI" {
		t.Fatalf("got %q want HI", buf)
	}
}

func TestHandshakePaddingEnabled(t *testing.T) {
	c, s := net.Pipe()
	defer c.Close()
	defer s.Close()
	scheme := &PaddingScheme{Enabled: true, MinLen: 8, MaxLen: 16}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := HandshakePadding(c, true, scheme); err != nil {
			t.Errorf("client: %v", err)
			return
		}
		if _, err := c.Write([]byte("DATA")); err != nil {
			t.Errorf("client write: %v", err)
		}
	}()
	if err := HandshakePadding(s, false, scheme); err != nil {
		t.Fatalf("server: %v", err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(s, buf); err != nil {
		t.Fatalf("read app data after padding: %v", err)
	}
	if string(buf) != "DATA" {
		t.Fatalf("got %q want DATA", buf)
	}
	<-done
}

func TestReadAndDiscardPaddingRejectsOversized(t *testing.T) {
	c, s := net.Pipe()
	defer c.Close()
	defer s.Close()
	go func() {
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], maxPaddingLen+1)
		s.Write(b[:])
	}()
	if err := readAndDiscardPadding(c); err == nil {
		t.Fatal("expected error for oversized padding, got nil")
	}
}

func TestRandomPaddingLen(t *testing.T) {
	scheme := &PaddingScheme{MinLen: 10, MaxLen: 20}
	seen := map[int]bool{}
	for i := 0; i < 200; i++ {
		n := randomPaddingLen(scheme)
		if n < 10 || n > 20 {
			t.Fatalf("randomPaddingLen = %d, want [10,20]", n)
		}
		seen[n] = true
	}
	if len(seen) < 2 {
		t.Fatalf("not random: %v", seen)
	}
}
