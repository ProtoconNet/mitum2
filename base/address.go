package base

import (
	"fmt"

	"github.com/ProtoconNet/mitum2/util"
	"github.com/ProtoconNet/mitum2/util/encoder"
	"github.com/pkg/errors"
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

	switch i, found := objcache.Get(s); {
	case !found:
	case i == nil:
		return nil, nil
	default:
		if err, ok := i.(error); ok {
			return nil, err
		}

		return i.(Address), nil //nolint:forcetypeassert //...
	}

	ad, err := decodeAddress(s, enc)
	if err != nil {
		err = errors.WithMessage(err, "address")
		objcache.Set(s, err, 0)

		return nil, err
	}

	objcache.Set(s, ad, 0)

	return ad, nil
}

func decodeAddress(s string, enc encoder.Encoder) (ad Address, _ error) {
	i, err := enc.DecodeWithFixedHintType(s, AddressTypeSize)

	switch {
	case err != nil:
		return nil, err
	case i == nil:
		return nil, nil
	default:
		if err := util.SetInterfaceValue(i, &ad); err != nil {
			return nil, err
		}

		return ad, nil
	}
}
