package launchcmd

import (
	"context"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/isaac"
	isaacblock "github.com/spikeekips/mitum/isaac/block"
	"github.com/spikeekips/mitum/launch"
	"github.com/spikeekips/mitum/util"
	"github.com/spikeekips/mitum/util/encoder"
	"github.com/spikeekips/mitum/util/logging"
	"github.com/spikeekips/mitum/util/ps"
)

var (
	PNameImportBlocks = ps.Name("import-blocks")
	PNameCheckStorage = ps.Name("check-blocks")
)

type ImportCommand struct { //nolint:govet //...
	// revive:disable:line-length-limit
	launch.DesignFlag
	Source          string           `arg:"" name:"source directory" help:"block data directory to import" type:"existingdir"`
	HeightRange     launch.RangeFlag `name:"range" help:"<from>-<to>" default:""`
	Vault           string           `name:"vault" help:"privatekey path of vault"`
	Do              bool             `name:"do" help:"really do import"`
	log             *zerolog.Logger
	launch.DevFlags `embed:"" prefix:"dev."`
	fromHeight      base.Height
	toHeight        base.Height
	prevblockmap    base.BlockMap
	// revive:enable:line-length-limit
}

func (cmd *ImportCommand) Run(pctx context.Context) error {
	var log *logging.Logging
	if err := util.LoadFromContextOK(pctx, launch.LoggingContextKey, &log); err != nil {
		return err
	}

	cmd.fromHeight, cmd.toHeight = base.NilHeight, base.NilHeight

	if h := cmd.HeightRange.From(); h != nil {
		cmd.fromHeight = base.Height(*h)

		if err := cmd.fromHeight.IsValid(nil); err != nil {
			return errors.WithMessagef(err, "invalid from height; from=%d", *h)
		}
	}

	if h := cmd.HeightRange.To(); h != nil {
		cmd.toHeight = base.Height(*h)

		if err := cmd.toHeight.IsValid(nil); err != nil {
			return errors.WithMessagef(err, "invalid to height; to=%d", *h)
		}

		if cmd.fromHeight > cmd.toHeight {
			return errors.Errorf("from height is higher than to; from=%d to=%d", cmd.fromHeight, cmd.toHeight)
		}
	}

	log.Log().Debug().
		Interface("design", cmd.DesignFlag).
		Interface("vault", cmd.Vault).
		Interface("dev", cmd.DevFlags).
		Str("source", cmd.Source).
		Interface("from_height", cmd.fromHeight).
		Interface("to_height", cmd.toHeight).
		Bool("do", cmd.Do).
		Msg("flags")

	cmd.log = log.Log()

	//revive:disable:modifies-parameter
	pctx = context.WithValue(pctx, launch.DesignFlagContextKey, cmd.DesignFlag)
	pctx = context.WithValue(pctx, launch.DevFlagsContextKey, cmd.DevFlags)
	pctx = context.WithValue(pctx, launch.VaultContextKey, cmd.Vault)
	//revive:enable:modifies-parameter

	pps := launch.DefaultImportPS()
	_ = pps.SetLogging(log)

	_ = pps.AddOK(PNameImportBlocks, cmd.importBlocks, nil, launch.PNameStorage)

	cmd.log.Debug().Interface("process", pps.Verbose()).Msg("process ready")

	pctx, err := pps.Run(pctx) //revive:disable-line:modifies-parameter
	defer func() {
		cmd.log.Debug().Interface("process", pps.Verbose()).Msg("process will be closed")

		if _, err = pps.Close(pctx); err != nil {
			cmd.log.Error().Err(err).Msg("failed to close")
		}
	}()

	return err
}

func (cmd *ImportCommand) importBlocks(pctx context.Context) (context.Context, error) {
	e := util.StringErrorFunc("failed to import blocks")

	var encs *encoder.Encoders
	var enc encoder.Encoder
	var design launch.NodeDesign
	var local base.LocalNode
	var params *isaac.LocalParams
	var db isaac.Database

	if err := util.LoadFromContextOK(pctx,
		launch.EncodersContextKey, &encs,
		launch.EncoderContextKey, &enc,
		launch.DesignContextKey, &design,
		launch.LocalContextKey, &local,
		launch.LocalParamsContextKey, &params,
		launch.CenterDatabaseContextKey, &db,
	); err != nil {
		return pctx, e(err, "")
	}

	var last base.Height

	switch i, err := cmd.checkHeights(pctx); {
	case err != nil:
		return pctx, e(err, "")
	default:
		last = i
	}

	if cmd.fromHeight > base.GenesisHeight {
		switch i, found, err := db.BlockMap(cmd.fromHeight - 1); {
		case err != nil:
			return pctx, err
		case !found:
			return pctx, errors.Errorf("previous blockmap not found for from height, %d", cmd.fromHeight-1)
		default:
			cmd.prevblockmap = i
		}
	}

	if err := cmd.validateSourceBlocks(last, enc, params); err != nil {
		return pctx, e(err, "")
	}

	if !cmd.Do {
		cmd.log.Debug().Msg("to import really blocks, `--do`")

		return pctx, nil
	}

	if err := launch.ImportBlocks(
		cmd.Source,
		design.Storage.Base,
		cmd.fromHeight,
		last,
		encs,
		enc,
		db,
		params,
	); err != nil {
		return pctx, e(err, "")
	}

	if err := cmd.validateImported(last, enc, design, params, db); err != nil {
		return pctx, e(err, "")
	}

	return pctx, nil
}

func (cmd *ImportCommand) checkHeights(pctx context.Context) (base.Height, error) {
	var last base.Height

	var encs *encoder.Encoders
	var enc encoder.Encoder
	var params *isaac.LocalParams
	var db isaac.Database

	if err := util.LoadFromContextOK(pctx,
		launch.EncodersContextKey, &encs,
		launch.EncoderContextKey, &enc,
		launch.LocalParamsContextKey, &params,
		launch.CenterDatabaseContextKey, &db,
	); err != nil {
		return last, err
	}

	switch fromHeight, toHeight, i, err := checkLastHeight(pctx, cmd.Source, cmd.fromHeight, cmd.toHeight); {
	case err != nil:
		return last, err
	default:
		cmd.fromHeight = fromHeight
		cmd.toHeight = toHeight
		last = i

		cmd.log.Debug().
			Interface("from_height", cmd.fromHeight).
			Interface("to_height", cmd.toHeight).
			Interface("last", last).
			Msg("heights checked")
	}

	switch i, found, err := db.LastBlockMap(); {
	case err != nil:
		return last, err
	case !found:
	case cmd.fromHeight < base.GenesisHeight:
		cmd.fromHeight = i.Manifest().Height() + 1
	case i.Manifest().Height() != cmd.fromHeight-1:
		return last, errors.Errorf(
			"from height should be same with last height + 1; from=%d last=%d", cmd.fromHeight, i.Manifest().Height())
	}

	cmd.log.Debug().
		Interface("from_height", cmd.fromHeight).
		Interface("to_height", cmd.toHeight).
		Interface("last", last).
		Msg("heights checked")

	return last, nil
}

func (cmd *ImportCommand) validateSourceBlocks(
	last base.Height,
	enc encoder.Encoder,
	params *isaac.LocalParams,
) error {
	e := util.StringErrorFunc("failed to validate source blocks")

	d := last - cmd.fromHeight

	if err := util.BatchWork(
		context.Background(),
		uint64(d.Int64())+1,
		333, //nolint:gomnd //...
		func(context.Context, uint64) error {
			return nil
		},
		func(_ context.Context, i, _ uint64) error {
			height := base.Height(int64(i) + cmd.fromHeight.Int64())

			return isaacblock.ValidateBlockFromLocalFS(height, cmd.Source, enc, params.NetworkID(), nil, nil, nil)
		},
	); err != nil {
		return e(err, "")
	}

	cmd.log.Debug().Msg("source blocks validated")

	return nil
}

func (cmd *ImportCommand) validateImported(
	last base.Height,
	enc encoder.Encoder,
	design launch.NodeDesign,
	params *isaac.LocalParams,
	db isaac.Database,
) error {
	e := util.StringErrorFunc("failed to validate imported")

	root := launch.LocalFSDataDirectory(design.Storage.Base)

	if err := isaacblock.ValidateBlocksFromStorage(root, cmd.fromHeight, last, enc, params, db, nil); err != nil {
		return e(err, "")
	}

	cmd.log.Debug().Msg("imported blocks validated")

	return nil
}

func checkLastHeight(pctx context.Context, source string, fromHeight, toHeight base.Height) (
	base.Height,
	base.Height,
	base.Height,
	error,
) {
	var last base.Height

	var encs *encoder.Encoders
	var enc encoder.Encoder
	var params *isaac.LocalParams
	var db isaac.Database

	if err := util.LoadFromContextOK(pctx,
		launch.EncodersContextKey, &encs,
		launch.EncoderContextKey, &enc,
		launch.LocalParamsContextKey, &params,
		launch.CenterDatabaseContextKey, &db,
	); err != nil {
		return fromHeight, toHeight, last, err
	}

	lastlocalheight := base.NilHeight

	switch i, found, err := db.LastBlockMap(); {
	case err != nil:
		return fromHeight, toHeight, last, err
	case !found:
	default:
		lastlocalheight = i.Manifest().Height()
	}

	if toHeight > base.NilHeight && toHeight <= lastlocalheight {
		return fromHeight, toHeight, last, errors.Errorf(
			"to height should be higher than last; to=%d last=%d", toHeight, lastlocalheight)
	}

	switch {
	case fromHeight < base.GenesisHeight:
		fromHeight = base.GenesisHeight //revive:disable-line:modifies-parameter
	case fromHeight > base.NilHeight:
		switch _, found, err := db.BlockMap(fromHeight - 1); {
		case err != nil:
			return fromHeight, toHeight, last, err
		case !found:
			return fromHeight, toHeight, last, errors.Errorf(
				"previous blockmap not found for from height, %d", fromHeight-1)
		}
	}

	switch i, found, err := isaacblock.FindLastHeightFromLocalFS(source, enc, params.NetworkID()); {
	case err != nil:
		return fromHeight, toHeight, last, err
	case !found, i < base.GenesisHeight:
		return fromHeight, toHeight, last, errors.Errorf("last height not found in source")
	case i < toHeight:
		return fromHeight, toHeight, last, errors.Errorf("last is lower than to height; last=%d to=%d", i, toHeight)
	case toHeight > base.NilHeight:
		last = toHeight
	default:
		last = i
	}

	switch {
	case fromHeight > last:
		return fromHeight, toHeight, last, errors.Errorf(
			"from height is higher than to; from=%d to=%d", fromHeight, last)
	case fromHeight < base.GenesisHeight:
		fromHeight = base.GenesisHeight //revive:disable-line:modifies-parameter
	}

	return fromHeight, toHeight, last, nil
}
