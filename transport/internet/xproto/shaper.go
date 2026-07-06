package xproto

import (
	"crypto/rand"
	"encoding/binary"
	"io"
	"net"

	"github.com/xtls/xray-core/common/dice"
	"github.com/xtls/xray-core/common/errors"
)

// maxPaddingLen caps a single padding exchange to bound resource use.
const maxPaddingLen = 1 << 16 // 64 KiB

// HandshakePadding performs a one-time bidirectional padding exchange right
// after the REALITY handshake, to blur the length signature of the first
// application packet (e.g. the VLESS header or inner TLS ClientHello).
//
// Protocol: the client writes [4-byte big-endian length N][N random bytes];
// the server reads and discards them, then writes its own padding; the client
// reads and discards it. After this the connection is fully transparent.
//
// This is a deliberately simple, transport-layer shaper. Continuous traffic
// morphing (a la anyTLS PaddingScheme) is incompatible with xproto's
// transparent transport and is left to the VLESS/Vision layer.
func HandshakePadding(conn net.Conn, isClient bool, scheme *PaddingScheme) error {
	if scheme == nil || !scheme.Enabled {
		return nil
	}
	padLen := randomPaddingLen(scheme)
	pad := make([]byte, 4+padLen)
	binary.BigEndian.PutUint32(pad, uint32(padLen))
	if _, err := rand.Read(pad[4:]); err != nil {
		return errors.New("xproto: padding rand failed").Base(err)
	}

	if isClient {
		if _, err := conn.Write(pad); err != nil {
			return err
		}
		return readAndDiscardPadding(conn)
	}
	if err := readAndDiscardPadding(conn); err != nil {
		return err
	}
	_, err := conn.Write(pad)
	return err
}

func readAndDiscardPadding(conn net.Conn) error {
	var lenBuf [4]byte
	if _, err := io.ReadFull(conn, lenBuf[:]); err != nil {
		return err
	}
	n := binary.BigEndian.Uint32(lenBuf[:])
	if n > maxPaddingLen {
		return errors.New("xproto: padding length ", n, " exceeds cap ", maxPaddingLen)
	}
	if n == 0 {
		return nil
	}
	_, err := io.ReadFull(conn, make([]byte, n))
	return err
}

func randomPaddingLen(scheme *PaddingScheme) int {
	if scheme == nil || scheme.MinLen >= scheme.MaxLen {
		return int(scheme.MinLen)
	}
	return int(scheme.MinLen) + dice.Roll(int(scheme.MaxLen-scheme.MinLen)+1)
}
