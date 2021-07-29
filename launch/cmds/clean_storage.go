package cmds

import (
	"context"
	"time"

	"golang.org/x/xerrors"

	"github.com/spikeekips/mitum/launch/pm"
	"github.com/spikeekips/mitum/launch/process"
	"github.com/spikeekips/mitum/storage"
	"github.com/spikeekips/mitum/storage/blockdata"
	"github.com/spikeekips/mitum/util"
)

type CleanStorageCommand struct {
	*BaseRunCommand
}

func NewCleanStorageCommand(dryrun bool) CleanStorageCommand {
	cmd := CleanStorageCommand{
		BaseRunCommand: NewBaseRunCommand(dryrun, "clean-storage"),
	}

	ps := cmd.Processes()
	if ps == nil {
		panic(xerrors.Errorf("processes not prepared"))
	}

	disabledProcessors := []string{
		process.ProcessNameProposalProcessor,
		process.ProcessNameConsensusStates,
		process.ProcessNameNetwork,
		process.ProcessNameSuffrage,
	}

	for i := range disabledProcessors {
		if err := ps.RemoveProcess(disabledProcessors[i]); err != nil {
			panic(err)
		}
	}

	hooks := []pm.Hook{
		pm.NewHook(pm.HookPrefixPre, process.ProcessNameLocalNode,
			"clean-storage", cmd.cleanStorage),
		pm.NewHook(pm.HookPrefixPre, process.ProcessNameGenerateGenesisBlock,
			process.HookNameCheckGenesisBlock, nil),
		pm.NewHook(pm.HookPrefixPost, process.ProcessNameConfig,
			process.HookNameConfigGenesisOperations, nil).SetOverride(true),
	}

	for i := range hooks {
		hook := hooks[i]
		if err := hook.Add(ps); err != nil {
			panic(err)
		}
	}

	_ = cmd.SetProcesses(ps)

	return cmd
}

func (cmd *CleanStorageCommand) Run(version util.Version) error {
	if err := cmd.Initialize(cmd, version); err != nil {
		return xerrors.Errorf("failed to initialize command: %w", err)
	}
	defer cmd.Done()
	defer func() {
		<-time.After(time.Second * 1)
	}()

	cmd.Log().Info().Bool("dryrun", cmd.dryrun).Msg("dryrun?")

	if err := cmd.prepare(); err != nil {
		return err
	}

	ps := cmd.Processes()
	_ = cmd.SetProcesses(ps)

	if cmd.dryrun {
		return nil
	}

	return ps.Run()
}

func (cmd *CleanStorageCommand) cleanStorage(ctx context.Context) (context.Context, error) {
	var st storage.Database
	if err := process.LoadDatabaseContextValue(ctx, &st); err != nil {
		return ctx, err
	}

	var blockData blockdata.BlockData
	if err := process.LoadBlockDataContextValue(ctx, &blockData); err != nil {
		return ctx, err
	}

	if err := blockdata.Clean(st, blockData, true); err != nil {
		return ctx, err
	}

	cmd.Log().Info().Msg("database and block data was cleaned")

	return ctx, nil
}
