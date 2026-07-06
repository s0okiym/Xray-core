// Package xproto is a private anti-censorship transport that wraps the
// REALITY handshake (github.com/xtls/reality) as a standalone transport,
// rather than a security layer stacked on top of tcp/splithttp/grpc.
//
// MVP: it reuses transport/internet/reality's *Config verbatim as its
// ProtocolSettings, so the handshake is identical to REALITY today. Private
// extensions (dest pool, key rotation, control stream, shaper) are layered on
// top in later stages without changing the handshake core.
package xproto

import (
	"github.com/xtls/xray-core/transport/internet"
	"github.com/xtls/xray-core/transport/internet/reality"
)

// ProtocolName is the registered transport protocol name.
const ProtocolName = "xproto"

// ConfigFromStreamSettings extracts the REALITY config that xproto rides on.
// Unlike REALITY (a security layer kept in SecuritySettings), xproto is an
// independent transport and keeps its config in the ProtocolSettings slot.
func ConfigFromStreamSettings(settings *internet.MemoryStreamConfig) *reality.Config {
	if settings == nil {
		return nil
	}
	config, ok := settings.ProtocolSettings.(*reality.Config)
	if !ok {
		return nil
	}
	return config
}
