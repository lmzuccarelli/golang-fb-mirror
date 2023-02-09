package mirror

import (
	"context"
	"io"
)

type MirrorInterface interface {
	Run(ctx context.Context, src, dest string, opts *CopyOptions, stdout io.Writer) (retErr error)
}

type Mirror struct{}

func New() MirrorInterface {
	return &Mirror{}
}

func (o *Mirror) Run(ctx context.Context, src, dest string, opts *CopyOptions, stdout io.Writer) (retErr error) {
	return Run(ctx, src, dest, opts, stdout)
}
