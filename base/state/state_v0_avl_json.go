package state

import (
	"encoding/json"

	jsonenc "github.com/spikeekips/mitum/util/encoder/json"
)

type StateV0AVLNodePackerJSON struct {
	jsonenc.HintedHead
	H   []byte   `json:"hash"`
	K   []byte   `json:"key"`
	HT  int16    `json:"height"`
	LF  []byte   `json:"left_key"`
	LFH []byte   `json:"left_hash"`
	RG  []byte   `json:"right_key"`
	RGH []byte   `json:"right_hash"`
	ST  *StateV0 `json:"state"`
}

func (stav StateV0AVLNode) MarshalJSON() ([]byte, error) {
	return jsonenc.Marshal(StateV0AVLNodePackerJSON{
		HintedHead: jsonenc.NewHintedHead(stav.Hint()),
		H:          stav.h,
		K:          stav.Key(),
		HT:         stav.height,
		LF:         stav.left,
		LFH:        stav.leftHash,
		RG:         stav.right,
		RGH:        stav.rightHash,
		ST:         stav.state,
	})
}

type StateV0AVLNodeUnpackerJSON struct {
	H   []byte          `json:"hash"`
	HT  int16           `json:"height"`
	LF  []byte          `json:"left_key"`
	LFH []byte          `json:"left_hash"`
	RG  []byte          `json:"right_key"`
	RGH []byte          `json:"right_hash"`
	ST  json.RawMessage `json:"state"`
}

func (stav *StateV0AVLNode) UnpackJSON(b []byte, enc *jsonenc.Encoder) error {
	var us StateV0AVLNodeUnpackerJSON
	if err := enc.Unmarshal(b, &us); err != nil {
		return err
	}

	return stav.unpack(enc, us.H, us.HT, us.LF, us.LFH, us.RG, us.RGH, us.ST)
}
