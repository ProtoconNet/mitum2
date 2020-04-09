package base

import (
	"fmt"

	"github.com/spikeekips/mitum/util"
	"github.com/spikeekips/mitum/util/hint"
	"github.com/spikeekips/mitum/util/isvalid"
	"github.com/spikeekips/mitum/util/logging"
)

// Address represents the address of account.
type Address interface {
	fmt.Stringer
	isvalid.IsValider
	hint.Hinter
	util.Byter
	logging.LogHintedMarshaler
	Equal(Address) bool
}