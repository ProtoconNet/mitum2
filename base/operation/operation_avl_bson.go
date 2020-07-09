package operation

import (
	"go.mongodb.org/mongo-driver/bson"

	bsonenc "github.com/spikeekips/mitum/util/encoder/bson"
)

func (em OperationAVLNode) MarshalBSON() ([]byte, error) {
	return bsonenc.Marshal(bsonenc.MergeBSONM(
		bsonenc.NewHintedDoc(em.Hint()),
		bson.M{
			"key":        em.key,
			"height":     em.height,
			"left_key":   em.left,
			"left_hash":  em.leftHash,
			"right_key":  em.right,
			"right_hash": em.rightHash,
			"hash":       em.h,
			"operation":  em.op,
		},
	))
}

type OperationAVLNodeUnpackerBSON struct {
	K   []byte   `bson:"key"`
	HT  int16    `bson:"height"`
	LF  []byte   `bson:"left_key"`
	LFH []byte   `bson:"left_hash"`
	RG  []byte   `bson:"right_key"`
	RGH []byte   `bson:"right_hash"`
	H   []byte   `bson:"hash"`
	OP  bson.Raw `bson:"operation"`
}

func (em *OperationAVLNode) UnpackBSON(b []byte, enc *bsonenc.Encoder) error {
	var ue OperationAVLNodeUnpackerBSON
	if err := enc.Unmarshal(b, &ue); err != nil {
		return err
	}

	return em.unpack(enc, ue.K, ue.HT, ue.LF, ue.LFH, ue.RG, ue.RGH, ue.H, ue.OP)
}
