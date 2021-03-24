package isaac

import (
	"context"

	"github.com/spikeekips/mitum/base/block"
	"github.com/spikeekips/mitum/base/prprocessor"
	"github.com/spikeekips/mitum/storage/blockdata"
	"github.com/spikeekips/mitum/util"
	"golang.org/x/xerrors"
)

func (pp *DefaultProcessor) Save(ctx context.Context) error {
	pp.Lock()
	defer pp.Unlock()

	if err := pp.resetSave(); err != nil {
		return err
	}

	pp.setState(prprocessor.Saving)

	if err := pp.save(ctx); err != nil {
		pp.setState(prprocessor.SaveFailed)

		if err0 := pp.resetSave(); err0 != nil {
			return err0
		}

		return err
	} else {
		pp.setState(prprocessor.Saved)

		return nil
	}
}

func (pp *DefaultProcessor) save(ctx context.Context) error {
	pp.Log().Debug().Msg("trying to save")

	if pp.preSaveHook != nil {
		if err := pp.preSaveHook(ctx); err != nil {
			return err
		}
	}

	sctx := ctx
	for _, f := range []func(context.Context) (context.Context, error){
		pp.storeBlockDataSession,
		pp.storeStorage,
	} {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if i, err := f(sctx); err != nil {
				pp.Log().Error().Err(err).Msg("failed to save")

				return err
			} else {
				sctx = i
			}
		}
	}

	if pp.postSaveHook != nil {
		if err := pp.postSaveHook(ctx); err != nil {
			return err
		}
	}

	pp.Log().Debug().Msg("saved")

	return nil
}

func (pp *DefaultProcessor) storeBlockDataSession(ctx context.Context) (context.Context, error) {
	pp.Log().Debug().Msg("trying to store block storage session")

	if pp.blockDataSession == nil {
		return ctx, xerrors.Errorf("not prepared")
	}

	if bd, err := pp.blockData.SaveSession(pp.blockDataSession); err != nil {
		pp.Log().Error().Err(err).Msg("trying to store block storage session")

		return ctx, err
	} else {
		ctx = context.WithValue(ctx, blockDataMapContextKey, bd)
	}

	pp.Log().Debug().Msg("stored block storage session")

	return ctx, nil
}

func (pp *DefaultProcessor) storeStorage(ctx context.Context) (context.Context, error) {
	pp.Log().Debug().Msg("trying to store storage")

	var bd block.BlockDataMap
	if err := util.LoadFromContextValue(ctx, blockDataMapContextKey, &bd); err != nil {
		return ctx, xerrors.Errorf("block data map not found: %w", err)
	}

	if err := pp.ss.Commit(ctx, bd); err != nil {
		pp.Log().Error().Err(err).Msg("failed to store storage")

		return ctx, err
	} else if err := pp.ss.Close(); err != nil {
		return ctx, err
	} else {
		pp.ss = nil

		pp.Log().Debug().Msg("stored storage")

		return ctx, nil
	}
}

func (pp *DefaultProcessor) resetSave() error {
	switch pp.state {
	case prprocessor.BeforePrepared,
		prprocessor.Preparing,
		prprocessor.PrepareFailed,
		prprocessor.Prepared,
		prprocessor.Canceled:

		pp.setState(prprocessor.BeforePrepared)

		return nil
	}

	pp.Log().Debug().Str("state", pp.state.String()).Msg("save will be resetted")

	if err := blockdata.CleanByHeight(pp.st, pp.blockData, pp.proposal.Height()); err != nil {
		return err
	}

	pp.setState(prprocessor.Prepared)

	return nil
}
