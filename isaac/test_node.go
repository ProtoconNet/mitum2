//go:build test
// +build test

package isaac

import "github.com/ProtoconNet/mitum2/base"

func RandomLocalNode() LocalNode {
	return NewLocalNode(base.NewMPrivatekey(), base.RandomAddress("local-"))
}
