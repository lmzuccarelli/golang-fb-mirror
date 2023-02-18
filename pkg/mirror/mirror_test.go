package mirror

import (
	"bufio"
	"context"
	"os"
	"testing"

	"github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/types"
)

func TestMirror(t *testing.T) {

	global := &GlobalOptions{Debug: true, TlsVerify: false, InsecurePolicy: true}

	_, sharedOpts := SharedImageFlags()
	_, deprecatedTLSVerifyOpt := DeprecatedTLSVerifyFlags()
	_, srcOpts := ImageFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	_, destOpts := ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	_, retryOpts := RetryFlags()
	opts := CopyOptions{
		Global:              global,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOpts,
		DestImage:           destOpts,
		RetryOpts:           retryOpts,
		Destination:         "oci:test",
		Dev:                 false,
		Mode:                mirrorToDisk,
		MultiArch:           "all",
		Format:              "oci",
		SignPassphraseFile:  "test-digest",
	}

	mm := &mockMirror{}
	m := New(mm)

	writer := bufio.NewWriter(os.Stdout)
	t.Run("Testing Worker : should pass", func(t *testing.T) {
		err := m.Run(context.Background(), "docker://localhost.localdomain:5000/test", "oci:test", &opts, *writer)
		if err != nil {
			t.Fatal("should pass")
		}
	})
}

// mock

type mockMirror struct{}

func (o *mockMirror) CopyImages(ctx context.Context, pc *signature.PolicyContext, destRef, srcRef types.ImageReference, opts *copy.Options) ([]byte, error) {
	return []byte("test"), nil
}
