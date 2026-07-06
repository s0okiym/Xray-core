package conf

import (
	"github.com/xtls/xray-core/app/dynconfig"
	"google.golang.org/protobuf/proto"
)

// DynConfigConfig is the JSON config for the DynConfig app. It seeds the
// initial dest pool; runtime updates happen via the commander API.
type DynConfigConfig struct {
	DestPool []string `json:"destPool"`
}

// Build implements Buildable.
func (c *DynConfigConfig) Build() (proto.Message, error) {
	return &dynconfig.Config{DestPool: c.DestPool}, nil
}
