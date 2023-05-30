//go:build test
// +build test

package isaacstates

import (
	"github.com/ProtoconNet/mitum2/base"
	isaacblock "github.com/ProtoconNet/mitum2/isaac/block"
	"github.com/ProtoconNet/mitum2/util"
	jsonenc "github.com/ProtoconNet/mitum2/util/encoder/json"
	"github.com/ProtoconNet/mitum2/util/valuehash"
)

func (st *baseHandler) setTimers(t *util.Timers) *baseHandler {
	st.timers = t

	return nil
}

func newTestBlockMap(
	height base.Height,
	previous, previousSuffrage util.Hash,
	local base.LocalNode,
	networkID base.NetworkID,
) (m isaacblock.BlockMap, _ error) {
	m = isaacblock.NewBlockMap(isaacblock.LocalFSWriterHint, jsonenc.JSONEncoderHint)

	for _, i := range []base.BlockMapItemType{
		base.BlockMapItemTypeProposal,
		base.BlockMapItemTypeOperations,
		base.BlockMapItemTypeOperationsTree,
		base.BlockMapItemTypeStates,
		base.BlockMapItemTypeStatesTree,
		base.BlockMapItemTypeVoteproofs,
	} {
		if err := m.SetItem(newTestBlockMapItem(i)); err != nil {
			return m, err
		}
	}

	if previous == nil {
		previous = valuehash.RandomSHA256()
	}
	if height != base.GenesisHeight && previousSuffrage == nil {
		previousSuffrage = valuehash.RandomSHA256()
	}

	manifest := base.NewDummyManifest(height, valuehash.RandomSHA256())
	manifest.SetPrevious(previous)
	manifest.SetSuffrage(previousSuffrage)

	m.SetManifest(manifest)
	err := m.Sign(local.Address(), local.Privatekey(), networkID)

	return m, err
}

func newTestBlockMapItem(t base.BlockMapItemType) isaacblock.BlockMapItem {
	return isaacblock.NewLocalBlockMapItem(t, util.UUID().String(), 1)
}
