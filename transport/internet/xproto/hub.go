package xproto

import (
	"context"
	"strings"
	"time"

	"github.com/xtls/xray-core/common"
	"github.com/xtls/xray-core/common/dice"
	"github.com/xtls/xray-core/common/errors"
	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/transport/internet"
	"github.com/xtls/xray-core/transport/internet/reality"
	"github.com/xtls/xray-core/transport/internet/stat"
)

// Listener is an internet.Listener that accepts TCP connections and runs the
// REALITY server handshake on each, rotating the fallback dest per connection
// when a dest pool is configured.
type Listener struct {
	listener net.Listener
	config   *Config
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

	l := &Listener{
		addConn: handler,
		config:  config,
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

// pickDest returns the fallback dest for one accepted connection. When a dest
// pool is configured, a random entry is chosen; otherwise the fixed base.dest
// is used. This is the first private extension over plain REALITY.
func (v *Listener) pickDest() string {
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
