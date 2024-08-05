package base

import (
	"sort"
	"strings"

	"github.com/ProtoconNet/mitum2/util"
)

type Node interface {
	util.HashByter
	util.IsValider
	Address() Address
	Publickey() Publickey
}

type LocalNode interface {
	Node
	Privatekey() Privatekey
}

func IsEqualNode(a, b Node) bool {
	switch {
	case a == nil || b == nil:
		return false
	case !a.Address().Equal(b.Address()):
		return false
	case !a.Publickey().Equal(b.Publickey()):
		return false
	default:
		return true
	}
}

func IsEqualNodes(a, b []Node) bool {
	switch {
	case len(a) < 1 && len(b) < 1:
		return true
	case len(a) != len(b):
		return false
	}

	sort.Slice(a, func(i, j int) bool {
		return strings.Compare(
			a[i].Address().String(),
			a[j].Address().String(),
		) < 0
	})

	sort.Slice(b, func(i, j int) bool {
		return strings.Compare(
			b[i].Address().String(),
			b[j].Address().String(),
		) < 0
	})

	for i := range a {
		if !IsEqualNode(a[i], b[i]) {
			return false
		}
	}

	return true
}
