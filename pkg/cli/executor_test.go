package services

import (
	"context"
	"fmt"
	"testing"

	"github.com/lmzuccarelli/golang-oci-mirror/pkg/api/v1alpha2"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/config"
	clog "github.com/lmzuccarelli/golang-oci-mirror/pkg/log"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/mirror"
	"github.com/spf13/cobra"
)

func TestExecutor(t *testing.T) {

	log := clog.New("trace")

	global := &mirror.GlobalOptions{
		TlsVerify:      false,
		InsecurePolicy: true,
		Force:          true,
	}
	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	_, srcOpts := mirror.ImageFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	_, destOpts := mirror.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	_, retryOpts := mirror.RetryFlags()

	opts := mirror.CopyOptions{
		Global:              global,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOpts,
		DestImage:           destOpts,
		RetryOpts:           retryOpts,
		Dev:                 false,
		Mode:                mirrorToDisk,
	}

	// read the ImageSetConfiguration
	cfg, err := config.ReadConfig(opts.Global.ConfigPath)
	if err != nil {
		log.Error("imagesetconfig %v ", err)
	}
	log.Debug("imagesetconfig : %v", cfg)

	// this test should cover over 80%

	t.Run("Testing Executor : should pass", func(t *testing.T) {
		collector := &Collector{Log: log, Config: cfg, Opts: opts, Fail: false}
		batch := &Batch{Log: log, Config: cfg, Opts: opts}
		ex := &ExecutorSchema{
			Log:              log,
			Config:           cfg,
			Opts:             opts,
			Operator:         collector,
			Release:          collector,
			AdditionalImages: collector,
			Batch:            batch,
		}

		res := &cobra.Command{}
		res.SetContext(context.Background())
		res.SilenceUsage = true
		ex.Opts.Mode = "mirrorToDisk"
		err := ex.Run(res, []string{"oci://test"})
		if err != nil {
			log.Error(" %v ", err)
			t.Fatalf("should not fail")
		}
	})

	t.Run("Testing Executor : should fail (batch worker)", func(t *testing.T) {
		collector := &Collector{Log: log, Config: cfg, Opts: opts, Fail: false}
		batch := &Batch{Log: log, Config: cfg, Opts: opts, Fail: true}
		ex := &ExecutorSchema{
			Log:              log,
			Config:           cfg,
			Opts:             opts,
			Operator:         collector,
			Release:          collector,
			AdditionalImages: collector,
			Batch:            batch,
		}

		res := &cobra.Command{}
		res.SilenceUsage = true
		res.SetContext(context.Background())
		ex.Opts.Mode = "mirrorToDisk"
		err := ex.Run(res, []string{"docker://test"})
		if err == nil {
			t.Fatalf("should fail")
		}
	})

	t.Run("Testing Executor : should fail (release collector)", func(t *testing.T) {
		releaseCollector := &Collector{Log: log, Config: cfg, Opts: opts, Fail: true}
		operatorCollector := &Collector{Log: log, Config: cfg, Opts: opts, Fail: false}
		batch := &Batch{Log: log, Config: cfg, Opts: opts, Fail: false}
		ex := &ExecutorSchema{
			Log:              log,
			Config:           cfg,
			Opts:             opts,
			Operator:         operatorCollector,
			Release:          releaseCollector,
			AdditionalImages: releaseCollector,
			Batch:            batch,
		}

		res := &cobra.Command{}
		res.SilenceUsage = true
		res.SetContext(context.Background())
		ex.Opts.Mode = "mirrorToDisk"
		err := ex.Run(res, []string{"oci://test"})
		if err == nil {
			t.Fatalf("should fail")
		}
	})

	t.Run("Testing Executor : should fail (operator collector)", func(t *testing.T) {
		releaseCollector := &Collector{Log: log, Config: cfg, Opts: opts, Fail: false}
		operatorCollector := &Collector{Log: log, Config: cfg, Opts: opts, Fail: true}
		batch := &Batch{Log: log, Config: cfg, Opts: opts, Fail: false}
		ex := &ExecutorSchema{
			Log:              log,
			Config:           cfg,
			Opts:             opts,
			Operator:         operatorCollector,
			Release:          releaseCollector,
			AdditionalImages: releaseCollector,
			Batch:            batch,
		}

		res := &cobra.Command{}
		res.SilenceUsage = true
		res.SetContext(context.Background())
		ex.Opts.Mode = "mirrorToDisk"
		err := ex.Run(res, []string{"oci://test"})
		if err == nil {
			t.Fatalf("should fail")
		}
	})

	t.Run("Testing Executor : should pass", func(t *testing.T) {
		ex := &ExecutorSchema{
			Log:    log,
			Config: cfg,
			Opts:   opts,
		}
		res := NewMirrorCmd(log)
		res.SilenceUsage = true
		ex.Opts.Global.ConfigPath = "hello"
		err := ex.Validate([]string{"oci://test"})
		if err != nil {
			log.Error(" %v ", err)
			t.Fatalf("should not fail")
		}
	})

	t.Run("Testing Executor : should fail", func(t *testing.T) {
		ex := &ExecutorSchema{
			Log:    log,
			Config: cfg,
			Opts:   opts,
		}
		res := NewMirrorCmd(log)
		res.SilenceUsage = true
		err := ex.Validate([]string{"test"})
		if err == nil {
			t.Fatalf("should fail")
		}
	})
}

// setup mocks

// for this test scenario we only need to mock
// ReleaseImageCollector, OperatorImageCollector and Batchr
type Collector struct {
	Log    clog.PluggableLoggerInterface
	Config v1alpha2.ImageSetConfiguration
	Opts   mirror.CopyOptions
	Fail   bool
}

type Batch struct {
	Log    clog.PluggableLoggerInterface
	Config v1alpha2.ImageSetConfiguration
	Opts   mirror.CopyOptions
	Fail   bool
}

func (o *Batch) Worker(ctx context.Context, images []string, opts mirror.CopyOptions) error {
	if o.Fail {
		return fmt.Errorf("forced error")
	}
	return nil
}

func (o *Collector) OperatorImageCollector(ctx context.Context) ([]string, error) {
	if o.Fail {
		return []string{}, fmt.Errorf("forced error operator collector")
	}
	test := []string{
		"docker://registry/name/namespace/sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea*oci:test",
		"docker://registry/name/namespace/sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea*oci:test",
		"docker://registry/name/namespace/sometestimage-c@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea*oci:test",
		"docker://registry/name/namespace/sometestimage-d@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea*oci:test",
		"docker://registry/name/namespace/sometestimage-ea@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea*oci:test",
	}
	return test, nil
}

func (o *Collector) ReleaseImageCollector(ctx context.Context) ([]string, error) {
	if o.Fail {
		return []string{}, fmt.Errorf("forced error release collector")
	}
	test := []string{
		"docker://registry/name/namespace/sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea*oci:test",
		"docker://registry/name/namespace/sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea*oci:test",
		"docker://registry/name/namespace/sometestimage-c@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea*oci:test",
		"docker://registry/name/namespace/sometestimage-d@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea*oci:test",
		"docker://registry/name/namespace/sometestimage-ea@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea*oci:test",
	}
	return test, nil
}

func (o *Collector) AdditionalImagesCollector(ctx context.Context) ([]string, error) {
	if o.Fail {
		return []string{}, fmt.Errorf("forced error release collector")
	}
	test := []string{
		"docker://registry/name/namespace/sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea*oci:test",
		"docker://registry/name/namespace/sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea*oci:test",
		"docker://registry/name/namespace/sometestimage-c@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea*oci:test",
		"docker://registry/name/namespace/sometestimage-d@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea*oci:test",
		"docker://registry/name/namespace/sometestimage-ea@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea*oci:test",
	}
	return test, nil
}
