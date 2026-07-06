package command

import (
	"context"

	"github.com/xtls/xray-core/common"
	core "github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/features/extension"
	"google.golang.org/grpc"
)

type service struct {
	UnimplementedDynConfigServiceServer
	v   *core.Instance
	dyn extension.DynConfig
}

func (s *service) GetDestPool(context.Context, *GetDestPoolRequest) (*GetDestPoolResponse, error) {
	return &GetDestPoolResponse{DestPool: s.dyn.DestPool()}, nil
}

func (s *service) SetDestPool(_ context.Context, req *SetDestPoolRequest) (*SetDestPoolResponse, error) {
	s.dyn.SetDestPool(req.DestPool)
	return &SetDestPoolResponse{}, nil
}

func (s *service) Register(server *grpc.Server) {
	RegisterDynConfigServiceServer(server, s)
}

func init() {
	common.Must(common.RegisterConfig((*Config)(nil), func(ctx context.Context, cfg interface{}) (interface{}, error) {
		s := core.MustFromContext(ctx)
		sv := &service{v: s}
		err := s.RequireFeatures(func(d extension.DynConfig) {
			sv.dyn = d
		}, false)
		if err != nil {
			return nil, err
		}
		return sv, nil
	}))
}
