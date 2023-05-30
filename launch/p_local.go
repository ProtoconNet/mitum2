package launch

import (
	"context"

	"github.com/ProtoconNet/mitum2/base"
	"github.com/ProtoconNet/mitum2/isaac"
	"github.com/ProtoconNet/mitum2/util"
	"github.com/ProtoconNet/mitum2/util/logging"
	"github.com/ProtoconNet/mitum2/util/ps"
)

var (
	PNameLocal            = ps.Name("local")
	LocalContextKey       = util.ContextKey("local")
	LocalParamsContextKey = util.ContextKey("local-params")
)

func PLocal(pctx context.Context) (context.Context, error) {
	e := util.StringErrorFunc("failed to load local")

	var log *logging.Logging
	if err := util.LoadFromContextOK(pctx, LoggingContextKey, &log); err != nil {
		return pctx, e(err, "")
	}

	var design NodeDesign
	if err := util.LoadFromContextOK(pctx, DesignContextKey, &design); err != nil {
		return pctx, e(err, "")
	}

	local, err := LocalFromDesign(design)
	if err != nil {
		return pctx, e(err, "")
	}

	log.Log().Info().Interface("local", local).Msg("local loaded")

	return context.WithValue(pctx, LocalContextKey, local), nil
}

func LocalFromDesign(design NodeDesign) (base.LocalNode, error) {
	local := isaac.NewLocalNode(design.Privatekey, design.Address)

	if err := local.IsValid(nil); err != nil {
		return nil, err
	}

	return local, nil
}
