package launch2

import (
	"context"

	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/isaac"
	isaacdatabase "github.com/spikeekips/mitum/isaac/database"
	"github.com/spikeekips/mitum/launch"
	"github.com/spikeekips/mitum/util"
	"github.com/spikeekips/mitum/util/encoder"
	"github.com/spikeekips/mitum/util/logging"
	"github.com/spikeekips/mitum/util/ps"
)

var PNameGenerateGenesis = ps.PName("generate-genesis")

func PGenerateGenesis(ctx context.Context) (context.Context, error) {
	e := util.StringErrorFunc("failed to generate genesis block")

	var log *logging.Logging
	var design launch.NodeDesign
	var genesisDesign launch.GenesisDesign
	var enc encoder.Encoder
	var local base.LocalNode
	var policy *isaac.NodePolicy
	var db isaac.Database
	var pool *isaacdatabase.TempPool

	if err := ps.LoadsFromContextOK(ctx,
		LoggingContextKey, &log,
		DesignContextKey, &design,
		GenesisDesignContextKey, &genesisDesign,
		EncoderContextKey, &enc,
		LocalContextKey, &local,
		NodePolicyContextKey, &policy,
		CenterDatabaseContextKey, &db,
		PoolDatabaseContextKey, &pool,
	); err != nil {
		return ctx, e(err, "")
	}

	g := launch.NewGenesisBlockGenerator(
		local,
		policy.NetworkID(),
		enc,
		db,
		pool,
		LocalFSDataDirectory(design.Storage.Base),
		genesisDesign.Facts,
	)
	_ = g.SetLogging(log)

	if _, err := g.Generate(); err != nil {
		return ctx, e(err, "")
	}

	return ctx, nil
}