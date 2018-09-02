package astiencoder

import (
	"context"
)

// CtxFunc executes a function and if everything went well checks whether the context has an error
func CtxFunc(ctx context.Context, fn func() error) (err error) {
	if err = fn(); err != nil {
		return err
	} else if ctx.Err() != nil {
		return ctx.Err()
	}
	return
}
