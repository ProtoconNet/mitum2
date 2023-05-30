package encoder

import (
	"io"
	"reflect"

	"github.com/ProtoconNet/mitum2/util"
	"github.com/ProtoconNet/mitum2/util/hint"
	"github.com/pkg/errors"
)

func Ptr(i interface{}) (ptr reflect.Value, elem reflect.Value) {
	elem = reflect.ValueOf(i)
	if elem.Type().Kind() == reflect.Ptr {
		return elem, elem.Elem()
	}

	if elem.CanAddr() {
		return elem.Addr(), elem
	}

	ptr = reflect.New(elem.Type())
	ptr.Elem().Set(elem)

	return ptr, elem
}

func AnalyzeSetHinter(d DecodeDetail, v interface{}) DecodeDetail {
	if _, ok := v.(hint.SetHinter); !ok {
		return d
	}

	oht := v.(hint.Hinter).Hint() //nolint:forcetypeassert //...

	// NOTE hint.BaseHinter
	var found bool
	if i, j := reflect.TypeOf(v).FieldByName("BaseHinter"); j && i.Type == reflect.TypeOf(hint.BaseHinter{}) {
		found = true
	}

	if !found {
		p := d.Decode
		d.Decode = func(b []byte, ht hint.Hint) (interface{}, error) {
			i, err := p(b, ht)
			if err != nil {
				return i, errors.WithMessage(err, "failed to decode")
			}

			if ht.IsEmpty() {
				ht = oht
			}

			return i.(hint.SetHinter).SetHint(ht), nil //nolint:forcetypeassert //...
		}

		return d
	}

	p := d.Decode
	d.Decode = func(b []byte, ht hint.Hint) (interface{}, error) {
		i, err := p(b, ht)
		if err != nil {
			return i, errors.WithMessage(err, "failed to decode")
		}

		n := reflect.New(reflect.TypeOf(i))
		n.Elem().Set(reflect.ValueOf(i))

		x := n.Elem().FieldByName("BaseHinter")
		if !x.IsValid() || !x.CanAddr() {
			return i, nil
		}

		if ht.IsEmpty() {
			ht = oht
		}

		x.Set(reflect.ValueOf(hint.NewBaseHinter(ht)))

		return n.Elem().Interface(), nil
	}

	return d
}

func Decode(enc Encoder, b []byte, v interface{}) error {
	e := util.StringErrorFunc("failed to decode")

	hinter, err := enc.Decode(b)
	if err != nil {
		return e(err, "")
	}

	if err := util.InterfaceSetValue(hinter, v); err != nil {
		return e(err, "")
	}

	return nil
}

func DecodeReader(enc Encoder, r io.Reader, v interface{}) error {
	e := util.StringErrorFunc("failed to decode from reader")

	b, err := io.ReadAll(r)
	if err != nil {
		return e(err, "")
	}

	if err := Decode(enc, b, v); err != nil {
		return e(err, "")
	}

	return nil
}
