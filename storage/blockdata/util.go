package blockdata

import (
	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/base/block"
	"github.com/spikeekips/mitum/storage"
	"github.com/spikeekips/mitum/util"
	"golang.org/x/xerrors"
)

// Clean makes Database and BlockData to be empty. If 'remove' is true, remove
// the BlockData directory itself.
func Clean(st storage.Database, blockData BlockData, remove bool) error {
	if err := st.Clean(); err != nil {
		return err
	} else if err := blockData.Clean(remove); err != nil {
		return err
	} else {
		return nil
	}
}

func CleanByHeight(st storage.Database, blockData BlockData, height base.Height) error {
	if err := st.LocalBlockDataMapsByHeight(height, func(bd block.BlockDataMap) (bool, error) {
		if err := blockData.RemoveAll(bd.Height()); err != nil {
			if xerrors.Is(err, util.NotFoundError) {
				return true, nil
			}

			return false, err
		} else {
			return true, nil
		}
	}); err != nil {
		return err
	}

	return st.CleanByHeight(height)
}

func CheckBlock(st storage.Database, blockData BlockData, networkID base.NetworkID) (block.Manifest, error) {
	var m block.Manifest
	if i, err := storage.CheckBlock(st, networkID); err != nil {
		return i, err
	} else {
		m = i
	}

	if found, err := blockData.Exists(m.Height()); err != nil {
		return m, err
	} else if !found {
		return m, util.NotFoundError.Errorf("block, %d not found in block data", m.Height())
	}

	return m, nil
}
