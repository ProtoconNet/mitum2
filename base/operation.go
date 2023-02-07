package base

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ProtoconNet/mitum2/util"
	"github.com/ProtoconNet/mitum2/util/encoder"
	"github.com/ProtoconNet/mitum2/util/fixedtree"
	"github.com/ProtoconNet/mitum2/util/hint"
	"github.com/ProtoconNet/mitum2/util/valuehash"
	"github.com/pkg/errors"
)

var (
	OperationFixedtreeHint              = hint.MustNewHint("operation-fixedtree-v0.0.1")
	BaseOperationProcessReasonErrorHint = hint.MustNewHint("operation-fixedtree-node-process-reason-v0.0.1")
)

type Operation interface {
	hint.Hinter
	util.IsValider
	util.Hasher
	SignFact
	PreProcess(context.Context, GetStateFunc) (context.Context, OperationProcessReasonError, error)
	Process(context.Context, GetStateFunc) ([]StateMergeValue, OperationProcessReasonError, error)
}

type (
	OperationProcessorProcessFunc    func(context.Context, Operation, GetStateFunc) (OperationProcessReasonError, error)
	NewOperationProcessorProcessFunc func(Height, GetStateFunc) (OperationProcessorProcessFunc, error)
)

var EmptyOperationProcessorProcessFunc = func(context.Context, Operation, GetStateFunc) (
	OperationProcessReasonError, error,
) {
	return nil, nil
}

type OperationProcessor interface {
	PreProcess(context.Context, Operation, GetStateFunc) (context.Context, OperationProcessReasonError, error)
	Process(context.Context, Operation, GetStateFunc) ([]StateMergeValue, OperationProcessReasonError, error)
	Close() error
}

type OperationFixedtreeNode struct {
	reason OperationProcessReasonError
	fixedtree.BaseNode
	inState bool
}

func newInStateOperationFixedtreeNode(facthash string, reason string, inState bool) OperationFixedtreeNode {
	var operr OperationProcessReasonError
	if len(reason) > 0 {
		operr = NewBaseOperationProcessReasonError(reason)
	}

	return OperationFixedtreeNode{
		BaseNode: fixedtree.NewBaseNode(facthash),
		inState:  inState,
		reason:   operr,
	}
}

func NewInStateOperationFixedtreeNode(facthash util.Hash, reason string) OperationFixedtreeNode {
	return newInStateOperationFixedtreeNode(facthash.String(), reason, true)
}

func NewNotInStateOperationFixedtreeNode(facthash util.Hash, reason string) OperationFixedtreeNode {
	return newInStateOperationFixedtreeNode(facthash.String()+"-", reason, false)
}

func (no OperationFixedtreeNode) InState() bool {
	_, instate := ParseTreeNodeOperationKey(no.Key())

	return instate
}

func (no OperationFixedtreeNode) Operation() util.Hash {
	h, _ := ParseTreeNodeOperationKey(no.Key())

	return h
}

func (no OperationFixedtreeNode) Reason() OperationProcessReasonError {
	return no.reason
}

func (no OperationFixedtreeNode) SetHash(h util.Hash) fixedtree.Node {
	return OperationFixedtreeNode{
		BaseNode: no.BaseNode.SetHash(h).(fixedtree.BaseNode), //nolint:forcetypeassert // ...
		inState:  no.inState,
		reason:   no.reason,
	}
}

type operationFixedtreeNodeJSONMarshaler struct {
	Reason OperationProcessReasonError `json:"reason"`
	fixedtree.BaseNodeJSONMarshaler
}

func (no OperationFixedtreeNode) MarshalJSON() ([]byte, error) {
	return util.MarshalJSON(operationFixedtreeNodeJSONMarshaler{
		BaseNodeJSONMarshaler: no.BaseNode.JSONMarshaler(),
		Reason:                no.reason,
	})
}

type operationFixedtreeNodeJSONUnmarshaler struct {
	Reason json.RawMessage `json:"reason"`
}

func (no *OperationFixedtreeNode) DecodeJSON(b []byte, enc encoder.Encoder) error {
	e := util.StringError("decode OperationFixedtreeNode")

	var ub fixedtree.BaseNode
	if err := enc.Unmarshal(b, &ub); err != nil {
		return e.Wrap(err)
	}

	no.BaseNode = ub

	var u operationFixedtreeNodeJSONUnmarshaler
	if err := enc.Unmarshal(b, &u); err != nil {
		return e.Wrap(err)
	}

	if err := encoder.Decode(enc, u.Reason, &no.reason); err != nil {
		return e.Wrap(err)
	}

	return nil
}

var ErrNotChangedOperationProcessReason = NewBaseOperationProcessReasonError( //nolint:varcheck // .
	"states not changed")

type OperationProcessReasonError interface {
	error
	Msg() string
}

type BaseOperationProcessReasonError struct {
	id  string
	msg string
	hint.BaseHinter
}

func NewBaseOperationProcessReasonError(s string, a ...interface{}) BaseOperationProcessReasonError {
	f := util.FuncCaller(3)

	return BaseOperationProcessReasonError{
		BaseHinter: hint.NewBaseHinter(BaseOperationProcessReasonErrorHint),
		id:         fmt.Sprintf("%+v", f),
		msg:        fmt.Errorf(s, a...).Error(), //nolint:goerr113 // it just stores error message
	}
}

func (BaseOperationProcessReasonError) Hint() hint.Hint {
	return BaseOperationProcessReasonErrorHint
}

func (e BaseOperationProcessReasonError) Is(err error) bool {
	b, eerr := util.AssertInterfaceValue[BaseOperationProcessReasonError](err)

	return eerr == nil && e.id == b.id
}

func (e BaseOperationProcessReasonError) Msg() string {
	return e.msg
}

func (e BaseOperationProcessReasonError) Error() string {
	return e.msg
}

type BaseOperationProcessReasonErrorJSONMarshaler struct {
	Message string `json:"message"`
	hint.BaseHinter
}

func (e BaseOperationProcessReasonError) MarshalJSON() ([]byte, error) {
	return util.MarshalJSON(BaseOperationProcessReasonErrorJSONMarshaler{
		BaseHinter: e.BaseHinter,
		Message:    e.msg,
	})
}

type BaseOperationProcessReasonErrorJSONUnmarshaler struct {
	Message string `json:"message"`
}

func (e *BaseOperationProcessReasonError) UnmarshalJSON(b []byte) error {
	var u BaseOperationProcessReasonErrorJSONUnmarshaler
	if err := util.UnmarshalJSON(b, &u); err != nil {
		return errors.Wrap(err, "unmarshal BaseOperationProcessReasonError")
	}

	e.msg = u.Message

	return nil
}

func ParseTreeNodeOperationKey(s string) (util.Hash, bool) {
	k := s

	var notInState bool

	if strings.HasSuffix(s, "-") {
		k = k[:len(k)-1]
		notInState = true
	}

	return valuehash.NewBytesFromString(k), !notInState
}

type BaseOperationProcessor struct {
	PreProcessConstraintFunc OperationProcessorProcessFunc
	ProcessConstraintFunc    OperationProcessorProcessFunc
	height                   Height
}

func NewBaseOperationProcessor(
	height Height,
	getStateFunc GetStateFunc,
	newPreProcessConstraintFunc NewOperationProcessorProcessFunc,
	newProcessConstraintFunc NewOperationProcessorProcessFunc,
) (*BaseOperationProcessor, error) {
	p := &BaseOperationProcessor{
		height: height,
	}

	switch {
	case newPreProcessConstraintFunc == nil:
		p.PreProcessConstraintFunc = EmptyOperationProcessorProcessFunc
	default:
		i, err := newPreProcessConstraintFunc(height, getStateFunc)
		if err != nil {
			return nil, err
		}

		p.PreProcessConstraintFunc = i
	}

	switch {
	case newProcessConstraintFunc == nil:
		p.ProcessConstraintFunc = EmptyOperationProcessorProcessFunc
	default:
		i, err := newProcessConstraintFunc(height, getStateFunc)
		if err != nil {
			return nil, err
		}

		p.ProcessConstraintFunc = i
	}

	return p, nil
}

func (p *BaseOperationProcessor) Close() error {
	p.PreProcessConstraintFunc = nil
	p.ProcessConstraintFunc = nil
	p.height = NilHeight

	return nil
}

func (p *BaseOperationProcessor) Height() Height {
	return p.height
}
