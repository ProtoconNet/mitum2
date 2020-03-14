package isaac

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/spikeekips/mitum/operation"
)

type testGenesisBlockV0 struct {
	baseTestStateHandler
	localstate *Localstate
}

func (t *testGenesisBlockV0) SetupTest() {
	t.baseTestStateHandler.SetupTest()
	baseLocalstate := t.baseTestStateHandler.localstate

	localstate, err := NewLocalstate(
		NewMemStorage(baseLocalstate.Storage().Encoders(), baseLocalstate.Storage().Encoder()),
		baseLocalstate.Node(),
		TestNetworkID,
	)
	t.NoError(err)
	t.localstate = localstate
}

func (t *testGenesisBlockV0) TestNewGenesisBlock() {
	op, err := NewKVOperation(
		t.localstate.Node().Privatekey(),
		[]byte("this-is-token"),
		"showme",
		[]byte("findme"),
		nil,
	)
	t.NoError(err)

	gg, err := NewGenesisBlockV0Generator(t.localstate, []operation.Operation{op})
	t.NoError(err)

	block, err := gg.Generate()
	t.NoError(err)

	t.Equal(Height(0), block.Height())
	t.Equal(Round(0), block.Round())

	pr, err := t.localstate.Storage().Seal(block.Proposal())
	t.NoError(err)
	t.NotNil(pr)

	st, found, err := t.localstate.Storage().State(op.Key)
	t.NoError(err)
	t.True(found)

	t.Equal(st.Key(), op.Key)
}

func TestGenesisBlockV0(t *testing.T) {
	suite.Run(t, new(testGenesisBlockV0))
}