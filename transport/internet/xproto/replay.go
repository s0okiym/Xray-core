package xproto

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"io"
	"net"
	"sync"
	"time"
)

// replayGuard tracks SHA256 hashes of recently seen ClientHello records and
// rejects exact replays within a ttl window. Legitimate connections always
// produce a fresh ClientHello (Random + key_share + sealed SessionId are all
// randomized per connection), so the guard never rejects them.
type replayGuard struct {
	mu   sync.Mutex
	seen map[[32]byte]time.Time
	ttl  time.Duration
}

func newReplayGuard(ttl time.Duration) *replayGuard {
	g := &replayGuard{seen: make(map[[32]byte]time.Time), ttl: ttl}
	go g.cleanupLoop()
	return g
}

// allow returns true if h is fresh (and records it), false if h was seen
// within the ttl window (i.e. a replay).
func (g *replayGuard) allow(h [32]byte) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if t, ok := g.seen[h]; ok && time.Since(t) < g.ttl {
		return false
	}
	g.seen[h] = time.Now()
	return true
}

func (g *replayGuard) cleanupLoop() {
	ticker := time.NewTicker(g.ttl)
	defer ticker.Stop()
	for range ticker.C {
		g.mu.Lock()
		now := time.Now()
		for h, t := range g.seen {
			if now.Sub(t) >= g.ttl {
				delete(g.seen, h)
			}
		}
		g.mu.Unlock()
	}
}

// clientHelloHash peeks the first TLS record (the ClientHello) from r and
// returns its SHA256 hash. Returns ok=false if the bytes are not a TLS
// handshake record or could not be read in full.
func clientHelloHash(r *bufio.Reader) (hash [32]byte, ok bool) {
	hdr, err := r.Peek(5)
	if err != nil || hdr[0] != 0x16 { // 0x16 = TLS handshake
		return [32]byte{}, false
	}
	recLen := int(binary.BigEndian.Uint16(hdr[3:5]))
	if recLen <= 0 || recLen > 1<<16 {
		return [32]byte{}, false
	}
	rec, err := r.Peek(5 + recLen)
	if err != nil {
		return [32]byte{}, false
	}
	return sha256.Sum256(rec), true
}

// peekConn wraps a net.Conn with a bufio.Reader so that bytes already peeked
// (the ClientHello) are returned on subsequent reads by reality.Server.
type peekConn struct {
	*bufio.Reader
	net.Conn
}

func (c *peekConn) Read(p []byte) (int, error) { return c.Reader.Read(p) }

// CloseWrite delegates to the underlying *net.TCPConn if supported. Required
// by reality.Server which casts the conn to CloseWriteConn.
func (c *peekConn) CloseWrite() error {
	if cw, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}
	return nil
}

// forwardToDest transparently forwards a connection to dest. Used for replay
// handling so the probe sees a real website response instead of a REALITY
// redirect (which would betray the proxy).
func forwardToDest(conn net.Conn, dest string) {
	target, err := net.Dial("tcp", dest)
	if err != nil {
		conn.Close()
		return
	}
	done := make(chan struct{}, 2)
	go func() { io.Copy(target, conn); done <- struct{}{} }()
	go func() { io.Copy(conn, target); done <- struct{}{} }()
	<-done
	target.Close()
	conn.Close()
	<-done
}
