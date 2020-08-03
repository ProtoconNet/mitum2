package state

import (
	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/util/encoder"
	"github.com/spikeekips/mitum/util/valuehash"
)

func (st *StateV0) unpack(
	enc encoder.Encoder,
	h valuehash.Hash,
	key string,
	bValue []byte,
	previousBlock valuehash.Hash,
	height base.Height,
	currentBlock valuehash.Hash,
	ops []valuehash.Bytes,
) error {
	if previousBlock.Empty() {
		previousBlock = nil
	}

	var value Value
	if v, err := DecodeValue(enc, bValue); err != nil {
		return err
	} else {
		value = v
	}

	uops := make([]valuehash.Hash, len(ops))
	for i := range ops {
		uops[i] = ops[i]
	}

	st.h = h
	st.key = key
	st.value = value
	st.previousBlock = previousBlock
	st.currentHeight = height
	st.currentBlock = currentBlock
	st.operations = uops

	return nil
}
