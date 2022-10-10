package util

import (
	"context"

	"github.com/pkg/errors"
)

type ContextKey string

func AwareContext(ctx context.Context, f func() error) error {
	errch := make(chan error, 1)

	go func() {
		errch <- f()
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errch:
		return err
	}
}

func LoadFromContextOK(ctx context.Context, a ...interface{}) error {
	if err := checkLoadFromContext(ctx, a...); err != nil {
		return err
	}

	return loadFromContext(ctx, load, a...)
}

func LoadFromContext(ctx context.Context, a ...interface{}) error {
	switch err := checkLoadFromContext(ctx, a...); {
	case err == nil:
	case errors.Is(err, ErrNotFound):
	default:
		return err
	}

	return loadFromContext(ctx, load, a...)
}

func checkLoadFromContext(ctx context.Context, a ...interface{}) error {
	switch {
	case len(a) < 1:
		return nil
	case len(a)%2 != 0:
		return errors.Errorf("should be, [key value] pairs")
	}

	for i := 0; i < len(a)/2; i++ {
		b := a[i*2]

		k, ok := b.(ContextKey)
		if !ok {
			return errors.Errorf("expected ContextKey, not %T", b)
		}

		if ctx.Value(k) == nil {
			return ErrNotFound.Errorf("key not found, %q", k)
		}
	}

	return nil
}

func loadFromContext(
	ctx context.Context,
	load func(context.Context, ContextKey, interface{}) error,
	a ...interface{},
) error {
	for i := 0; i < len(a)/2; i++ {
		b := a[i*2]

		v := a[i*2+1]

		if err := load(ctx, b.(ContextKey), v); err != nil { //nolint:forcetypeassert //...
			return err
		}
	}

	return nil
}

func load(ctx context.Context, key ContextKey, v interface{}) error {
	i := ctx.Value(key)
	if i == nil {
		return nil
	}

	if err := InterfaceSetValue(i, v); err != nil {
		return errors.WithMessagef(err, "failed to load value from context, %q", key)
	}

	return nil
}
