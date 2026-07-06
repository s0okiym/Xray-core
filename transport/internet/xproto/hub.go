package xproto

import (
	"context"
	"strings"
	"time"

	goreality "github.com/xtls/reality"
	"github.com/xtls/xray-core/common"
	"github.com/xtls/xray-core/common/errors"
	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/transport/internet"
	"github.com/xtls/xray-core/transport/internet/reality"
	"github.com/xtls/xray-core/transport/internet/stat"
)

// Listener is an internet.Listener that accepts TCP connections and runs the
// REALITY server handshake on each.
type Listener struct {
	listener      net.Listener
	realityConfig *goreality.Config
	addConn       internet.ConnHandler
}

// ListenXproto creates a new xproto Listener.
func ListenXproto(ctx context.Context, address net.Address, port net.Port, streamSettings *internet.MemoryStreamConfig, handler internet.ConnHandler) (internet.Listener, error) {
	if port == net.Port(0) {
		return nil, errors.New("xproto: unix listener is not supported").AtError()
	}
	config := ConfigFromStreamSettings(streamSettings)
	if config == nil {
		return nil, errors.New(`xproto: empty "xprotoSettings"`).AtError()
	}

	l := &Listener{
		addConn: handler,
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

	l.realityConfig = config.GetREALITYConfig()
	go goreality.DetectPostHandshakeRecordsLens(l.realityConfig)

	go l.keepAccepting()
	return l, nil
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
			if conn, err = reality.Server(conn, v.realityConfig); err != nil {
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
