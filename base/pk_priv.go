package base

import (
	"fmt"
	"strings"

	"github.com/btcsuite/btcutil/base58"

	"github.com/ProtoconNet/mitum2/util"
	"github.com/ProtoconNet/mitum2/util/hint"
	"github.com/ProtoconNet/mitum2/util/valuehash"
	btcec "github.com/btcsuite/btcd/btcec/v2"
	btcec_ecdsa "github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

var (
	MPrivatekeyHint = hint.MustNewHint("mpr-v0.0.1")
	MPublickeyHint  = hint.MustNewHint("mpu-v0.0.1")
)

const PrivatekeyMinSeedSize = 36

// MPrivatekey is the default privatekey of mitum, it is based on BTC Privatekey.
type MPrivatekey struct {
	priv *btcec.PrivateKey
	hint.BaseHinter
}

func NewMPrivatekey() *MPrivatekey {
	priv, _ := btcec.NewPrivateKey()

	return newMPrivatekeyFromPrivateKey(priv)
}

func NewMPrivatekeyFromSeed(s string) (*MPrivatekey, error) {
	if l := len([]byte(s)); l < PrivatekeyMinSeedSize {
		return nil, util.ErrInvalid.Errorf(
			"wrong seed for privatekey; too short, %d < %d", l, PrivatekeyMinSeedSize)
	}

	priv, _ := btcec.PrivKeyFromBytes(valuehash.NewSHA256([]byte(s)).Bytes())

	return newMPrivatekeyFromPrivateKey(priv), nil
}

func ParseMPrivatekey(s string) (*MPrivatekey, error) {
	t := MPrivatekeyHint.Type().String()

	switch {
	case !strings.HasSuffix(s, t):
		return nil, util.ErrInvalid.Errorf("unknown privatekey string")
	case len(s) <= len(t):
		return nil, util.ErrInvalid.Errorf("invalid privatekey string; too short")
	}

	return LoadMPrivatekey(s[:len(s)-len(t)])
}

func LoadMPrivatekey(s string) (*MPrivatekey, error) {
	b := base58.Decode(s)

	if len(b) < 1 {
		return nil, util.ErrInvalid.Errorf("malformed private key")
	}

	priv, _ := btcec.PrivKeyFromBytes(b)

	return newMPrivatekeyFromPrivateKey(priv), nil
}

func newMPrivatekeyFromPrivateKey(priv *btcec.PrivateKey) *MPrivatekey {
	return &MPrivatekey{
		BaseHinter: hint.NewBaseHinter(MPrivatekeyHint),
		priv:       priv,
	}
}

func (k *MPrivatekey) String() string {
	return fmt.Sprintf("%s%s", base58.Encode(k.priv.Serialize()), k.Hint().Type().String())
}

func (k *MPrivatekey) Bytes() []byte {
	return []byte(k.String())
}

func (k *MPrivatekey) IsValid([]byte) error {
	if err := k.BaseHinter.IsValid(MPrivatekeyHint.Type().Bytes()); err != nil {
		return util.ErrInvalid.WithMessage(err, "wrong hint in privatekey")
	}

	if k.priv == nil {
		return util.ErrInvalid.Errorf("empty btc privatekey")
	}

	return nil
}

func (k *MPrivatekey) Publickey() Publickey {
	return NewMPublickey(k.priv.PubKey())
}

func (k *MPrivatekey) Equal(b PKKey) bool {
	return IsEqualPKKey(k, b)
}

func (k *MPrivatekey) Sign(b []byte) (Signature, error) {
	sig := btcec_ecdsa.Sign(k.priv, chainhash.DoubleHashB(b))

	return Signature(sig.Serialize()), nil
}

func (k *MPrivatekey) MarshalText() ([]byte, error) {
	return k.Bytes(), nil
}

func (k *MPrivatekey) UnmarshalText(b []byte) error {
	u, err := LoadMPrivatekey(string(b))
	if err != nil {
		return err
	}

	*k = *u

	return nil
}
