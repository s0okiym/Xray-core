// Package xproto is a private anti-censorship transport that wraps the
// REALITY handshake (github.com/xtls/reality) as a standalone transport,
// rather than a security layer stacked on top of tcp/splithttp/grpc.
//
// Stage 2: xproto carries its own Config (embedding a REALITY config plus a
// private dest pool). The handshake core is still REALITY; the dest pool is
// the first private extension layered on top.
package xproto

import "github.com/xtls/xray-core/transport/internet"

// ProtocolName is the registered transport protocol name.
const ProtocolName = "xproto"

// ConfigFromStreamSettings extracts the xproto config from the stream settings'
// transport (ProtocolSettings) slot.
func ConfigFromStreamSettings(settings *internet.MemoryStreamConfig) *Config {
	if settings == nil {
		return nil
	}
	config, ok := settings.ProtocolSettings.(*Config)
	if !ok {
		return nil
	}
	return config
}
