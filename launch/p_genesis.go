package launch

import (
	"context"

	"github.com/ProtoconNet/mitum2/base"
	"github.com/ProtoconNet/mitum2/isaac"
	"github.com/ProtoconNet/mitum2/util"
	"github.com/ProtoconNet/mitum2/util/encoder"
	"github.com/ProtoconNet/mitum2/util/logging"
	"github.com/ProtoconNet/mitum2/util/ps"
)

var PNameGenerateGenesis = ps.Name("generate-genesis")

func PGenerateGenesis(pctx context.Context) (context.Context, error) {
	e := util.StringErrorFunc("failed to generate genesis block")

	var log *logging.Logging
	var design NodeDesign
	var genesisDesign GenesisDesign
	var enc encoder.Encoder
	var local base.LocalNode
	var params *isaac.LocalParams
	var db isaac.Database

	if err := util.LoadFromContextOK(pctx,
		LoggingContextKey, &log,
		DesignContextKey, &design,
		GenesisDesignContextKey, &genesisDesign,
		EncoderContextKey, &enc,
		LocalContextKey, &local,
		LocalParamsContextKey, &params,
		CenterDatabaseContextKey, &db,
	); err != nil {
		return pctx, e(err, "")
	}

	g := NewGenesisBlockGenerator(
		local,
		params.NetworkID(),
		enc,
		db,
		LocalFSDataDirectory(design.Storage.Base),
		genesisDesign.Facts,
	)
	_ = g.SetLogging(log)

	if _, err := g.Generate(); err != nil {
		return pctx, e(err, "")
	}

	return pctx, nil
}
