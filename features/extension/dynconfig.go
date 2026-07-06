package extension

import "github.com/xtls/xray-core/features"

// DynConfig is a feature that holds dynamic anti-censorship parameters (dest
// pool, etc.) that can be updated at runtime via the commander API without
// restarting the instance. Consumed by transports (e.g. xproto) that want to
// rotate parameters dynamically.
type DynConfig interface {
	features.Feature

	// DestPool returns a snapshot copy of the current dest pool.
	DestPool() []string

	// SetDestPool replaces the dest pool at runtime (via commander).
	SetDestPool(pool []string)
}

// DynConfigType returns the feature type used to look up DynConfig via
// Instance.GetFeature.
func DynConfigType() interface{} {
	return (*DynConfig)(nil)
}
