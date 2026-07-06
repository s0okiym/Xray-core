// Package dynconfig is a feature that holds dynamic anti-censorship parameters
// (dest pool, etc.) updatable at runtime via the commander API, without
// restarting the instance. Consumed by transports (e.g. xproto) that rotate
// parameters dynamically.
package dynconfig

import (
	"context"
	"sync"

	"github.com/xtls/xray-core/common"
	"github.com/xtls/xray-core/features/extension"
)

// DynConfig is the runtime holder of dynamic parameters. Stage 3 implements
// dest pool rotation; server_keys / fingerprint_pool / padding_scheme are
// reserved for later stages.
type DynConfig struct {
	sync.RWMutex
	destPool []string
}

// Type implements features.Feature.
func (d *DynConfig) Type() interface{} { return extension.DynConfigType() }

// Start implements features.Feature.
func (d *DynConfig) Start() error { return nil }

// Close implements features.Feature.
func (d *DynConfig) Close() error { return nil }

// DestPool returns a snapshot copy of the current dest pool.
func (d *DynConfig) DestPool() []string {
	d.RLock()
	defer d.RUnlock()
	return append([]string(nil), d.destPool...)
}

// SetDestPool replaces the dest pool at runtime. Called via commander.
func (d *DynConfig) SetDestPool(pool []string) {
	d.Lock()
	defer d.Unlock()
	d.destPool = append([]string(nil), pool...)
}

func init() {
	common.Must(common.RegisterConfig((*Config)(nil), func(ctx context.Context, config interface{}) (interface{}, error) {
		c := config.(*Config)
		d := &DynConfig{destPool: append([]string(nil), c.DestPool...)}
		return d, nil
	}))
}
