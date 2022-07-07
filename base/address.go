package base

import (
	"fmt"

	"github.com/spikeekips/mitum/util"
	"github.com/spikeekips/mitum/util/encoder"
)

const AddressTypeSize = 3

// Address represents the address of account.
type Address interface {
	fmt.Stringer // NOTE String() should be typed string
	util.Byter
	util.IsValider
	Equal(Address) bool
}

type Addresses []Address

func (as Addresses) Len() int {
	return len(as)
}

func (as Addresses) Less(i, j int) bool {
	return as[i].String() < as[j].String()
}

func (as Addresses) Swap(i, j int) {
	as[i], as[j] = as[j], as[i]
}

// DecodeAddress decodes Address from string.
func DecodeAddress(s string, enc encoder.Encoder) (Address, error) {
	if len(s) < 1 {
		return nil, nil
	}

	cachekey := s + "/" + enc.Hint().String()
	if i, found := objcache.Get(cachekey); found {
		return i.(Address), nil //nolint:forcetypeassert //...
	}

	e := util.StringErrorFunc("failed to parse address")

	i, err := enc.DecodeWithFixedHintType(s, AddressTypeSize)

	switch {
	case err != nil:
		return nil, e(err, "failed to decode address")
	case i == nil:
		objcache.Set(cachekey, nil, nil)

		return nil, nil
	}

	ad, ok := i.(Address)
	if !ok {
		return nil, e(nil, "failed to decode address; not Address, %T", i)
	}

	objcache.Set(cachekey, ad, nil)

	return ad, nil
}
