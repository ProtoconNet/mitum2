//go:build test
// +build test

package base

import "github.com/ProtoconNet/mitum2/util"

func RandomAddress(prefix string) Address {
	return NewStringAddress(prefix + util.UUID().String())
}
