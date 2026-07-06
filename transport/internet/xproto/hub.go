package xproto

import (
	"context"
	"strings"
	"time"

	goreality "github.com/xtls/reality"
	"github.com/xtls/xray-core/common"
	"github.com/xtls/xray-core/common/dice"
	"github.com/xtls/xray-core/common/errors"
	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/features/extension"
	"github.com/xtls/xray-core/transport/internet"
	"github.com/xtls/xray-core/transport/internet/reality"
	"github.com/xtls/xray-core/transport/internet/stat"
)

// Listener is an internet.Listener that accepts TCP connections and runs the
// REALITY server handshake on each, rotating the fallback dest per connection
// from a dynamic pool (DynConfig feature) or the static config dests.
type Listener struct {
	listener net.Listener
	config   *Config
	dyn      extension.DynConfig // may be nil
	addConn  internet.ConnHandler
}

// ListenXproto creates a new xproto Listener.
func ListenXproto(ctx context.Context, address net.Address, port net.Port, streamSettings *internet.MemoryStreamConfig, handler internet.ConnHandler) (internet.Listener, error) {
	if port == net.Port(0) {
		return nil, errors.New("xproto: unix listener is not supported").AtError()
	}
	config := ConfigFromStreamSettings(streamSettings)
	if config == nil || config.Base == nil {
		return nil, errors.New(`xproto: empty "xprotoSettings" or missing base reality config`).AtError()
	}

	l := &Listener{addConn: handler, config: config}
	// reality.Server's handshake waits on GlobalPostHandshakeRecordsLens,
	// populated by DetectPostHandshakeRecordsLens. Start it for each dest in
	// the pool (or the fixed base.dest), otherwise the handshake hangs.
	detectionDests := config.Dests
	if len(detectionDests) == 0 && config.Base.Dest != "" {
		detectionDests = []string{config.Base.Dest}
	}
	for _, dest := range detectionDests {
		rc := *config.Base
		rc.Dest = dest
		go goreality.DetectPostHandshakeRecordsLens(rc.GetREALITYConfig())
	}
	// Look up the DynConfig feature (optional). When present, the server rotates
	// the fallback dest from its dynamic pool instead of the static config.
	if v := core.FromContext(ctx); v != nil {
		if f := v.GetFeature(extension.DynConfigType()); f != nil {
			l.dyn, _ = f.(extension.DynConfig)
		}
	}

	listener, err := internet.ListenSystem(ctx, &net.TCPAddr{
		IP:   address.IP(),
		Port: int(port),
	}, streamSettings.SocketSettings)
	if err != nil {
		return nil, errors.New("xproto: failed to listen TCP on ", address, ":", port).Base(err)
	}
	errors.LogInfo(ctx, "xproto: listening TCP on ", address, ":", port)
	l.listener = listener

	go l.keepAccepting()
	return l, nil
}

// pickDest returns the fallback dest for one accepted connection. Precedence:
// (1) dynamic pool from the DynConfig feature, (2) static dests pool in config,
// (3) fixed base.dest.
func (v *Listener) pickDest() string {
	if v.dyn != nil {
		if pool := v.dyn.DestPool(); len(pool) > 0 {
			return pool[dice.Roll(len(pool))]
		}
	}
	if len(v.config.Dests) > 0 {
		return v.config.Dests[dice.Roll(len(v.config.Dests))]
	}
	return v.config.Base.Dest
}

func (v *Listener) keepAccepting() {
	for {
		conn, err := v.listener.Accept()
		if err != nil {
			errStr := err.Error()
			if strings.Contains(errStr, "closed") {
				break
			}
			errors.LogWarningInner(context.Background(), err, "xproto: failed to accept raw connections")
			if strings.Contains(errStr, "too many") {
				time.Sleep(time.Millisecond * 500)
			}
			continue
		}

		go func() {
			// Shallow-copy the base reality config so we can override Dest per
			// connection without racing other accept goroutines.
			rc := *v.config.Base
			if dest := v.pickDest(); dest != "" {
				rc.Dest = dest
			}
			gorealityCfg := rc.GetREALITYConfig()
			if conn, err = reality.Server(conn, gorealityCfg); err != nil {
				errors.LogInfo(context.Background(), err.Error())
				return
			}
			if v.config.Padding != nil && v.config.Padding.Enabled {
				if err := HandshakePadding(conn, false, v.config.Padding); err != nil {
					errors.LogInfo(context.Background(), "xproto: handshake padding failed: ", err)
					return
				}
			}
			v.addConn(stat.Connection(conn))
		}()
	}
}

// Addr implements internet.Listener.Addr.
func (v *Listener) Addr() net.Addr {
	return v.listener.Addr()
}

// Close implements internet.Listener.Close.
func (v *Listener) Close() error {
	return v.listener.Close()
}

func init() {
	common.Must(internet.RegisterTransportListener(ProtocolName, ListenXproto))
}
