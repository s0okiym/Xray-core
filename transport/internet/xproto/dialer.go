package xproto

import (
	"context"

	"github.com/xtls/xray-core/common"
	"github.com/xtls/xray-core/common/errors"
	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/transport/internet"
	"github.com/xtls/xray-core/transport/internet/reality"
	"github.com/xtls/xray-core/transport/internet/stat"
)

// Dial dials a new TCP connection to dest and runs the REALITY client
// handshake on top of it. The dest pool is a server-side concern and ignored
// here.
func Dial(ctx context.Context, dest net.Destination, streamSettings *internet.MemoryStreamConfig) (stat.Connection, error) {
	errors.LogInfo(ctx, "xproto: dialing to ", dest)
	conn, err := internet.DialSystem(ctx, dest, streamSettings.SocketSettings)
	if err != nil {
		return nil, err
	}
	config := ConfigFromStreamSettings(streamSettings)
	if config == nil || config.Base == nil {
		conn.Close()
		return nil, errors.New(`xproto: empty "xprotoSettings" or missing base reality config`).AtError()
	}
	if conn, err = reality.UClient(conn, config.Base, ctx, dest); err != nil {
		return nil, err
	}
	return stat.Connection(conn), nil
}

func init() {
	common.Must(internet.RegisterTransportDialer(ProtocolName, Dial))
}
