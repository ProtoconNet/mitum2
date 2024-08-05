package fixedtree

import (
	"testing"

	"github.com/ProtoconNet/mitum2/util"
	"github.com/ProtoconNet/mitum2/util/hint"
	"github.com/ProtoconNet/mitum2/util/valuehash"
	"github.com/stretchr/testify/suite"
)

type baseTestTree struct {
	suite.Suite
	ht hint.Hint
}

func (t *baseTestTree) SetupSuite() {
	t.ht = hint.MustNewHint("test-tree-v0.0.1")
}

func (t *baseTestTree) nodesWithoutHash(n int) []Node {
	nodes := make([]Node, n)
	for i := range nodes {
		node := NewBaseNode(util.UUID().String())

		nodes[i] = node
	}

	return nodes
}

func (t *baseTestTree) nodes(n int) []Node {
	nodes := t.nodesWithoutHash(n)
	nodes, err := generateNodesHash(nodes, func(uint64, Node) error { return nil })
	t.NoError(err)

	return nodes
}

type testTree struct {
	baseTestTree
}

func (t *testTree) TestNew() {
	nodes := t.nodes(33)

	tr, err := NewTree(t.ht, nodes)
	t.NoError(err)
	t.NoError(tr.IsValid(nil))
	t.Equal(len(nodes), tr.Len())

	tr.Traverse(func(index uint64, b Node) (bool, error) {
		a := nodes[index]

		t.True(a.Equal(b))

		return true, nil
	})
}

func (t *testTree) TestInvalid() {
	t.Run("invalid node", func() {
		nodes := t.nodes(3)

		n := nodes[0].(BaseNode)
		n.h = nil
		nodes[0] = n

		tr, err := NewTree(t.ht, nodes)
		t.NoError(err)
		err = tr.IsValid(nil)
		t.ErrorIs(err, util.ErrInvalid)
		t.ErrorContains(err, "empty hash")
	})

	t.Run("wrong node hash", func() {
		nodes := t.nodes(3)

		n := nodes[2].(BaseNode)
		n.h = valuehash.RandomSHA256()
		nodes[2] = n

		tr, err := NewTree(t.ht, nodes)
		t.NoError(err)
		err = tr.IsValid(nil)
		t.ErrorIs(err, util.ErrInvalid)
		t.ErrorContains(err, "hash does not match")
	})
}

func TestTree(t *testing.T) {
	suite.Run(t, new(testTree))
}
