package isaacblock

import (
	"testing"

	"github.com/ProtoconNet/mitum2/base"
	"github.com/ProtoconNet/mitum2/isaac"
	"github.com/ProtoconNet/mitum2/util/encoder"
	jsonenc "github.com/ProtoconNet/mitum2/util/encoder/json"
	"github.com/ProtoconNet/mitum2/util/hint"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/suite"
)

type testBlockReaders struct {
	suite.Suite
}

func (t *testBlockReaders) TestNew() {
	readers := NewBlockReaders()

	t.Run("unknown", func() {
		f := readers.Find(hint.MustNewHint("ab-v0.0.1"))
		t.Nil(f)
	})

	t.Run("known", func() {
		ht := hint.MustNewHint("abc-v0.0.1")

		t.NoError(readers.Add(ht, func(base.Height, encoder.Encoder) (isaac.BlockReader, error) { return nil, nil }))

		f := readers.Find(ht)
		t.NotNil(f)
	})

	t.Run("compatible", func() {
		ht := hint.MustNewHint("abc-v0.0.9")

		f := readers.Find(ht)
		t.NotNil(f)
	})

	t.Run("not compatible", func() {
		ht := hint.MustNewHint("abc-v1.0.1")

		f := readers.Find(ht)
		t.Nil(f)
	})
}

func (t *testBlockReaders) TestLoadReader() {
	encs := encoder.NewEncoders()
	enc := jsonenc.NewEncoder()
	t.NoError(encs.AddHinter(enc))

	readers := NewBlockReaders()

	writerhint := hint.MustNewHint("writer-v0.0.1")
	t.NoError(readers.Add(writerhint, func(base.Height, encoder.Encoder) (isaac.BlockReader, error) { return nil, errors.Errorf("findme") }))

	t.Run("known", func() {
		_, err := LoadBlockReader(readers, encs, writerhint, enc.Hint(), base.Height(66))
		t.Error(err)
		t.ErrorContains(err, "findme")
	})

	t.Run("unknown writer", func() {
		_, err := LoadBlockReader(readers, encs, hint.MustNewHint("hehe-v0.0.1"), enc.Hint(), base.Height(66))
		t.Error(err)
		t.ErrorContains(err, "unknown writer hint")
	})

	t.Run("unknown encodeer", func() {
		_, err := LoadBlockReader(readers, encs, writerhint, hint.MustNewHint("hehe-v0.0.1"), base.Height(66))
		t.Error(err)
		t.ErrorContains(err, "unknown encoder hint")
	})
}

func TestBlockReaders(t *testing.T) {
	suite.Run(t, new(testBlockReaders))
}
