package base

import "github.com/ProtoconNet/mitum2/util"

type HeightDecoder struct {
	h       Height
	decoded bool
}

func (d *HeightDecoder) UnmarshalJSON(b []byte) error {
	e := util.StringError("unmarshal height")

	var u Height
	if err := util.UnmarshalJSON(b, &u); err != nil {
		return e.Wrap(err)
	}

	d.h = u
	d.decoded = true

	return nil
}

func (d HeightDecoder) Height() Height {
	if !d.decoded {
		return NilHeight
	}

	return d.h
}
