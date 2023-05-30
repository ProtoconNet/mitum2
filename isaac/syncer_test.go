package isaac

import (
	"context"
	"testing"
	"time"

	"github.com/ProtoconNet/mitum2/base"
	"github.com/ProtoconNet/mitum2/network/quicmemberlist"
	"github.com/ProtoconNet/mitum2/network/quicstream"
	"github.com/ProtoconNet/mitum2/util"
	"github.com/lucas-clemente/quic-go"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/suite"
)

type dummyNodeConnInfo struct {
	quicmemberlist.NamedConnInfo
	base.BaseNode
}

func newDummyNodeConnInfo(address base.Address, pub base.Publickey, addr string, tlsinsecure bool) dummyNodeConnInfo {
	return dummyNodeConnInfo{
		BaseNode:      NewNode(pub, address),
		NamedConnInfo: quicmemberlist.NewNamedConnInfo(addr, tlsinsecure),
	}
}

func (n dummyNodeConnInfo) IsValid([]byte) error {
	e := util.ErrInvalid.Errorf("invalid dummyNodeConnInfo")

	if err := n.BaseNode.IsValid(nil); err != nil {
		return e.Wrap(err)
	}

	if err := n.NamedConnInfo.IsValid(nil); err != nil {
		return e.Wrap(err)
	}

	return nil
}

type testSyncSourcePool struct {
	suite.Suite
}

func (t *testSyncSourcePool) newnci() dummyNodeConnInfo {
	ci := quicstream.RandomConnInfo()

	return newDummyNodeConnInfo(
		base.RandomAddress(""),
		base.NewMPrivatekey().Publickey(),
		ci.Addr().String(),
		ci.TLSInsecure(),
	)
}

func (t *testSyncSourcePool) TestNew() {
	t.Run("ok", func() {
		sources := make([]NodeConnInfo, 3)

		for i := range sources {
			sources[i] = t.newnci()
		}

		p := NewSyncSourcePool(sources)

		t.NotEmpty(p.fixedids)
		t.NotEmpty(p.fixed)

		for i := range sources {
			nci := sources[i]
			t.True(p.IsInFixed(nci.Address()))
			rncis := p.NodeConnInfo(nci.Address())
			t.Equal(1, len(rncis))
			t.Equal(nci.String(), rncis[0].String())
		}
	})

	t.Run("empty", func() {
		p := NewSyncSourcePool(nil)

		t.Empty(p.fixedids)
		t.Empty(p.fixed)
	})
}

func (t *testSyncSourcePool) TestUpdate() {
	prevsources := make([]NodeConnInfo, 3)
	newsources := make([]NodeConnInfo, 3)

	for i := range prevsources {
		prevsources[i] = t.newnci()
		newsources[i] = t.newnci()
	}

	t.Run("update", func() {
		p := NewSyncSourcePool(prevsources)

		t.True(p.UpdateFixed(newsources))
		t.False(p.UpdateFixed(newsources))

		for i := range newsources {
			nci := newsources[i]
			t.True(p.IsInFixed(nci.Address()))
			rncis := p.NodeConnInfo(nci.Address())
			t.Equal(1, len(rncis))
			t.Equal(nci.String(), rncis[0].String())
		}
	})

	t.Run("update and reset", func() {
		p := NewSyncSourcePool(prevsources)

		t.True(p.UpdateFixed(newsources))

		nci, _, err := p.Pick()
		t.NoError(err)
		t.Equal(newsources[0].String(), nci.String())

		t.True(p.UpdateFixed(prevsources))

		nci, _, err = p.Pick()
		t.NoError(err)
		t.Equal(prevsources[0].String(), nci.String())
	})

	t.Run("update empty", func() {
		p := NewSyncSourcePool(prevsources)

		t.True(p.UpdateFixed(newsources))

		t.True(p.UpdateFixed(nil))

		_, _, err := p.Pick()
		t.Error(err)
		t.True(errors.Is(err, ErrEmptySyncSources))
	})

	t.Run("update and in nonfixed", func() {
		p := NewSyncSourcePool(prevsources)

		newci := newsources[0]

		t.True(p.AddNonFixed(newci))
		t.True(p.NodeExists(newci.Address()))
		t.False(p.IsInFixed(newci.Address()))
		t.True(p.IsInNonFixed(newci.Address()))

		t.True(p.UpdateFixed(newsources))
		t.True(p.NodeExists(newci.Address()))
		t.True(p.IsInFixed(newci.Address()))
		t.False(p.IsInNonFixed(newci.Address()))
	})
}

func (t *testSyncSourcePool) TestAdd() {
	sources := make([]NodeConnInfo, 3)

	for i := range sources {
		sources[i] = t.newnci()
	}

	t.Run("new", func() {
		p := NewSyncSourcePool(sources)

		added := t.newnci()
		t.True(p.AddNonFixed(added))

		t.False(p.IsInFixed(added.Address()))
		rncis := p.NodeConnInfo(added.Address())
		t.Equal(1, len(rncis))
		t.Equal(added.String(), rncis[0].String())
	})

	t.Run("known", func() {
		p := NewSyncSourcePool(sources)

		t.False(p.AddNonFixed(sources[1]))
	})

	t.Run("update in fixed", func() {
		p := NewSyncSourcePool(sources)

		prev := p.Len()

		newci := quicstream.RandomConnInfo()
		added := sources[1]
		added = newDummyNodeConnInfo(
			added.Address(),
			added.Publickey(),
			newci.Addr().String(),
			newci.TLSInsecure(),
		)

		t.True(p.AddNonFixed(added))

		t.Equal(prev+1, p.Len())

		rncis := p.NodeConnInfo(added.Address())
		t.Equal(2, len(rncis))
		t.Equal(sources[1].String(), rncis[0].String())
		t.Equal(added.String(), rncis[1].String())

		// remove by node
		t.True(p.RemoveNonFixedNode(added.Address()))
		rncis = p.NodeConnInfo(added.Address())
		t.Equal(1, len(rncis))
	})
}

func (t *testSyncSourcePool) TestRemove() {
	sources := make([]NodeConnInfo, 3)
	for i := range sources {
		sources[i] = t.newnci()
	}

	t.Run("ok", func() {
		p := NewSyncSourcePool(sources)

		added := make([]NodeConnInfo, 3)
		for i := range added {
			added[i] = t.newnci()
		}

		t.True(p.AddNonFixed(added...))

		t.True(p.RemoveNonFixed(added[0]))
		t.Equal(len(sources)+len(added)-1, p.Len())

		t.False(p.NodeExists(added[0].Address()))
	})

	t.Run("known in fixed", func() {
		p := NewSyncSourcePool(sources)

		added := make([]NodeConnInfo, 3)
		for i := range added {
			added[i] = t.newnci()
		}

		t.True(p.AddNonFixed(added...))

		t.False(p.RemoveNonFixed(sources[0]))
		t.Equal(len(sources)+len(added), p.Len())
		t.True(p.NodeExists(sources[0].Address()))
	})

	t.Run("multiple", func() {
		p := NewSyncSourcePool(sources)

		removenode := base.RandomAddress("")

		added := make([]NodeConnInfo, 4)
		for i := range added {
			newci := t.newnci()

			node := newci.Address()

			if i > 1 {
				node = removenode
			}

			added[i] = newDummyNodeConnInfo(
				node,
				newci.Publickey(),
				newci.Addr().String(),
				newci.TLSInsecure(),
			)
		}

		t.True(p.AddNonFixed(added...))

		t.Equal(len(sources)+len(added), p.Len())

		t.True(p.RemoveNonFixedNode(removenode))

		t.Equal(len(sources)+len(added)-2, p.Len())

		rncis := p.NodeConnInfo(removenode)
		t.Empty(rncis)
	})
}

func (t *testSyncSourcePool) TestSameID() {
	sources := make([]NodeConnInfo, 3)

	for i := range sources {
		sources[i] = t.newnci()
	}

	p0 := NewSyncSourcePool(sources)
	p1 := NewSyncSourcePool(sources)

	t.Equal(p0.fixedids, p1.fixedids)
}

func (t *testSyncSourcePool) TestNext() {
	sources := make([]NodeConnInfo, 3)

	for i := range sources {
		sources[i] = t.newnci()
	}

	p := NewSyncSourcePool(sources)

	uncis := make([]NodeConnInfo, len(sources))

	for i := range make([]struct{}, len(sources)) {
		nci, report, err := p.Pick()
		t.NoError(err)
		report(ErrRetrySyncSources.Call())

		uncis[i] = nci
	}

	for i := range uncis {
		a := sources[i]
		b := uncis[i]

		t.True(a.Address().Equal(b.Address()))
		t.True(a.Publickey().Equal(b.Publickey()))
		t.Equal(a.String(), b.String())
	}
}

func (t *testSyncSourcePool) TestRenew() {
	sources := make([]NodeConnInfo, 3)

	for i := range sources {
		sources[i] = t.newnci()
	}

	p := NewSyncSourcePool(sources)

	nci, report, err := p.Pick()
	t.NoError(err)
	report(ErrRetrySyncSources.Call())
	t.Equal(sources[0].String(), nci.String())

	nci, _, err = p.Pick()
	t.NoError(err)
	t.Equal(sources[1].String(), nci.String())

	p.problems.Purge()

	nci, _, err = p.Pick()
	t.NoError(err)
	t.Equal(sources[0].String(), nci.String())
}

func (t *testSyncSourcePool) TestNextButEmpty() {
	p := NewSyncSourcePool(nil)

	next, id, err := p.Pick()
	t.Error(err)
	t.True(errors.Is(err, ErrEmptySyncSources))
	t.Nil(next)
	t.Empty(id)
}

func (t *testSyncSourcePool) TestConcurrent() {
	sources := make([]NodeConnInfo, 3)

	for i := range sources {
		sources[i] = t.newnci()
	}

	p := NewSyncSourcePool(sources)
	p.renewTimeout = time.Millisecond * 10

	t.NoError(util.RunErrgroupWorker(context.Background(), 333, func(_ context.Context, i uint64, _ uint64) error {
		if i%3 == 0 {
			<-time.After(p.renewTimeout + 2)
		}

		nci, report, err := p.Pick()

		switch {
		case err != nil:
			if errors.Is(err, ErrEmptySyncSources) {
				return nil
			}

			return err
		case report == nil:
			return errors.Errorf("empty report")
		case nci == nil:
			return errors.Errorf("empty node conn info")
		default:
			t.T().Log("id", p.makeid(nci))

			if i%3 == 0 {
				report(nil)
			}

			return nil
		}
	}))
}

func (t *testSyncSourcePool) TestRetry() {
	sources := make([]NodeConnInfo, 3)

	for i := range sources {
		sources[i] = t.newnci()
	}

	t.Run("once", func() {
		p := NewSyncSourcePool(sources)

		var called int
		err := p.Retry(context.Background(), func(ci NodeConnInfo) (bool, error) {
			called++

			return false, nil
		}, 3, time.Millisecond*10)
		t.NoError(err)

		t.Equal(1, called)
	})

	t.Run("error once", func() {
		p := NewSyncSourcePool(sources)

		var called int
		err := p.Retry(context.Background(), func(ci NodeConnInfo) (bool, error) {
			called++

			if ci.Address().Equal(sources[0].Address()) {
				return false, errors.Errorf("hihihi")
			}

			return false, nil
		}, 3, time.Millisecond*10)
		t.Error(err)
		t.ErrorContains(err, "hihihi")

		t.Equal(1, called)
	})

	t.Run("ErrRetrySyncSources once", func() {
		p := NewSyncSourcePool(sources)

		var called int
		var last NodeConnInfo
		err := p.Retry(context.Background(), func(ci NodeConnInfo) (bool, error) {
			called++

			last = ci

			if ci.Address().Equal(sources[0].Address()) {
				return false, ErrRetrySyncSources.Errorf("hihihi")
			}

			return false, nil
		}, 3, time.Millisecond*10)
		t.NoError(err)

		t.Equal(2, called)

		next := sources[1]

		t.True(last.Address().Equal(next.Address()))
		t.True(last.Publickey().Equal(next.Publickey()))
		t.Equal(last.String(), next.String())
	})

	t.Run("network error once", func() {
		p := NewSyncSourcePool(sources)

		var called int
		var last NodeConnInfo
		err := p.Retry(context.Background(), func(ci NodeConnInfo) (bool, error) {
			called++

			last = ci

			if ci.Address().Equal(sources[0].Address()) {
				return false, &quic.StreamError{StreamID: 333, ErrorCode: quic.StreamErrorCode(444)}
			}

			return false, nil
		}, 3, time.Millisecond*10)
		t.NoError(err)

		t.Equal(2, called)

		next := sources[1]

		t.True(last.Address().Equal(next.Address()))
		t.True(last.Publickey().Equal(next.Publickey()))
		t.Equal(last.String(), next.String())
	})

	t.Run("long endure error", func() {
		p := NewSyncSourcePool(sources)

		var called int

		err := p.Retry(context.Background(), func(ci NodeConnInfo) (bool, error) {
			called++

			if called > 2 && ci.Address().Equal(sources[1].Address()) {
				return false, errors.Errorf("hihihi")
			}

			return false, &quic.StreamError{StreamID: 333, ErrorCode: quic.StreamErrorCode(444)}
		}, -1, time.Millisecond*10)
		t.Error(err)
		t.ErrorContains(err, "hihihi")

		t.Equal(len(sources)+2, called)
	})
}

func (t *testSyncSourcePool) TestPickMultiple() {
	sources := make([]NodeConnInfo, 3)

	for i := range sources {
		sources[i] = t.newnci()
	}

	t.Run("zero", func() {
		p := NewSyncSourcePool(sources)

		_, _, err := p.PickMultiple(0)
		t.Error(err)
		t.ErrorContains(err, "zero")
	})

	t.Run("one", func() {
		p := NewSyncSourcePool(sources)

		ncis, _, err := p.PickMultiple(1)
		t.NoError(err)
		t.Equal(1, len(ncis))
		t.Equal(sources[0].String(), ncis[0].String())
	})

	t.Run("two", func() {
		p := NewSyncSourcePool(sources)

		ncis, _, err := p.PickMultiple(2)
		t.NoError(err)
		t.Equal(2, len(ncis))

		for i := range ncis {
			t.Equal(sources[i].String(), ncis[i].String())
		}
	})

	t.Run("all", func() {
		p := NewSyncSourcePool(sources)

		ncis, _, err := p.PickMultiple(len(sources))
		t.NoError(err)
		t.Equal(len(sources), len(ncis))

		for i := range ncis {
			t.Equal(sources[i].String(), ncis[i].String(), i)
		}
	})

	t.Run("over size", func() {
		p := NewSyncSourcePool(sources)

		ncis, _, err := p.PickMultiple(len(sources) + 100)
		t.NoError(err)
		t.Equal(len(sources), len(ncis))

		for i := range ncis {
			t.Equal(sources[i].String(), ncis[i].String(), i)
		}
	})
}

func TestSyncSourcePool(tt *testing.T) {
	suite.Run(tt, new(testSyncSourcePool))
}
