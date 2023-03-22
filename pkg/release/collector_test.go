package release

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/lmzuccarelli/golang-fb-mirror/pkg/api/v1alpha2"
	"github.com/lmzuccarelli/golang-fb-mirror/pkg/api/v1alpha3"
	clog "github.com/lmzuccarelli/golang-fb-mirror/pkg/log"
	"github.com/lmzuccarelli/golang-fb-mirror/pkg/mirror"
)

func TestReleaseImageCollector(t *testing.T) {

	log := clog.New("trace")

	global := &mirror.GlobalOptions{
		TlsVerify:      false,
		InsecurePolicy: true,
		Dir:            "../../tests",
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
		Destination:         "oci://test",
		Dev:                 false,
		Mode:                mirrorToDisk,
	}

	cfg := v1alpha2.ImageSetConfiguration{
		ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
			Mirror: v1alpha2.Mirror{
				Platform: v1alpha2.Platform{
					Graph: true,
					Channels: []v1alpha2.ReleaseChannel{
						{
							Name: "stable-4.7",
						},
						{
							Name:       "stable-4.6",
							MinVersion: "4.6.3",
							MaxVersion: "4.6.13",
						},
						{
							Name: "okd",
							Type: v1alpha2.TypeOKD,
						},
					},
				},
				Operators: []v1alpha2.Operator{
					{
						Catalog: "redhat-operators:v4.7",
						Full:    true,
					},
					{
						Catalog: "certified-operators:v4.7",
						Full:    true,
						IncludeConfig: v1alpha2.IncludeConfig{
							Packages: []v1alpha2.IncludePackage{
								{Name: "couchbase-operator"},
								{
									Name: "mongodb-operator",
									IncludeBundle: v1alpha2.IncludeBundle{
										MinVersion: "1.4.0",
									},
								},
								{
									Name: "crunchy-postgresql-operator",
									Channels: []v1alpha2.IncludeChannel{
										{Name: "stable"},
									},
								},
							},
						},
					},
					{
						Catalog: "community-operators:v4.7",
					},
				},
				AdditionalImages: []v1alpha2.Image{
					{Name: "registry.redhat.io/ubi8/ubi:latest"},
				},
				Helm: v1alpha2.Helm{
					Repositories: []v1alpha2.Repository{
						{
							URL:  "https://stefanprodan.github.io/podinfo",
							Name: "podinfo",
							Charts: []v1alpha2.Chart{
								{Name: "podinfo", Version: "5.0.0"},
							},
						},
					},
					Local: []v1alpha2.Chart{
						{Name: "podinfo", Path: "/test/podinfo-5.0.0.tar.gz"},
					},
				},
				BlockedImages: []v1alpha2.Image{
					{Name: "alpine"},
					{Name: "redis"},
				},
				Samples: []v1alpha2.SampleImages{
					{Image: v1alpha2.Image{Name: "ruby"}},
					{Image: v1alpha2.Image{Name: "python"}},
					{Image: v1alpha2.Image{Name: "nginx"}},
				},
			},
		},
	}

	cincinnati := &Cincinnati{Config: cfg, Opts: opts}
	ctx := context.Background()

	// this test should cover over 80%
	t.Run("Testing ReleaseImageCollector : should pass", func(t *testing.T) {
		manifest := &Manifest{Log: log}
		ex := &Collector{
			Log:        log,
			Mirror:     &Mirror{Fail: false},
			Config:     cfg,
			Manifest:   manifest,
			Opts:       opts,
			Cincinnati: cincinnati,
		}
		res, err := ex.ReleaseImageCollector(ctx)
		if err != nil {
			t.Fatalf("should not fail")
		}
		log.Debug("completed test related images %v ", res)
	})

	t.Run("Testing ReleaseImageCollector : should fail mirror", func(t *testing.T) {
		os.RemoveAll("../../tests/hold-release/")
		manifest := &Manifest{Log: log}
		ex := &Collector{
			Log:        log,
			Mirror:     &Mirror{Fail: true},
			Config:     cfg,
			Manifest:   manifest,
			Opts:       opts,
			Cincinnati: cincinnati,
		}
		res, err := ex.ReleaseImageCollector(ctx)
		if err == nil {
			t.Fatalf("should fail")
		}
		log.Debug("completed test related images %v ", res)
	})

	t.Run("Testing ReleaseImageCollector : should fail image index", func(t *testing.T) {
		manifest := &Manifest{Log: log, FailImageIndex: true}
		ex := &Collector{
			Log:        log,
			Mirror:     &Mirror{Fail: false},
			Config:     cfg,
			Manifest:   manifest,
			Opts:       opts,
			Cincinnati: cincinnati,
		}
		res, err := ex.ReleaseImageCollector(ctx)
		if err == nil {
			t.Fatalf("should fail")
		}
		log.Debug("completed test related images %v ", res)
	})

	t.Run("Testing ReleaseImageCollector : should fail image manifest", func(t *testing.T) {
		manifest := &Manifest{Log: log, FailImageManifest: true}
		ex := &Collector{
			Log:        log,
			Mirror:     &Mirror{Fail: false},
			Config:     cfg,
			Manifest:   manifest,
			Opts:       opts,
			Cincinnati: cincinnati,
		}
		res, err := ex.ReleaseImageCollector(ctx)
		if err == nil {
			t.Fatalf("should fail")
		}
		log.Debug("completed test related images %v ", res)
	})

	t.Run("Testing ReleaseImageCollector : should fail extract", func(t *testing.T) {
		manifest := &Manifest{Log: log, FailExtract: true}
		ex := &Collector{
			Log:        log,
			Mirror:     &Mirror{Fail: false},
			Config:     cfg,
			Manifest:   manifest,
			Opts:       opts,
			Cincinnati: cincinnati,
		}
		res, err := ex.ReleaseImageCollector(ctx)
		if err == nil {
			t.Fatalf("should fail")
		}
		log.Debug("completed test related images %v ", res)
	})
}

// setup mocks
// we need to mock Manifest, Mirror, Cincinnati

type Mirror struct {
	Fail bool
}

type Manifest struct {
	Log               clog.PluggableLoggerInterface
	FailImageIndex    bool
	FailImageManifest bool
	FailExtract       bool
}

type Cincinnati struct {
	Config v1alpha2.ImageSetConfiguration
	Opts   mirror.CopyOptions
	Client Client
	Fail   bool
}

func (o *Mirror) Run(ctx context.Context, src, dest, mode string, opts *mirror.CopyOptions, out bufio.Writer) error {
	if o.Fail {
		return fmt.Errorf("forced mirror run fail")
	}
	return nil
}

func (o *Manifest) GetOperatorConfig(file string) (*v1alpha3.OperatorConfigSchema, error) {
	return nil, nil
}

func (o *Manifest) GetRelatedImagesFromCatalogByFilter(filePath, label string, op v1alpha2.Operator, mp map[string]v1alpha3.ISCPackage) (map[string][]v1alpha3.RelatedImage, error) {
	return nil, nil
}

func (o *Manifest) GetReleaseSchema(filePath string) ([]v1alpha3.RelatedImage, error) {
	relatedImages := []v1alpha3.RelatedImage{
		{Name: "testA", Image: "sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
		{Name: "testB", Image: "sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
		{Name: "testC", Image: "sometestimage-c@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
		{Name: "testD", Image: "sometestimage-d@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
	}
	return relatedImages, nil
}

func (o *Manifest) GetImageIndex(name string) (*v1alpha3.OCISchema, error) {
	if o.FailImageIndex {
		return &v1alpha3.OCISchema{}, fmt.Errorf("forced error image index")
	}
	return &v1alpha3.OCISchema{
		SchemaVersion: 2,
		Manifests: []v1alpha3.OCIManifest{
			{
				MediaType: "application/vnd.oci.image.manifest.v1+json",
				Digest:    "sha256:3ef0b0141abd1548f60c4f3b23ecfc415142b0e842215f38e98610a3b2e52419",
				Size:      567,
			},
		},
	}, nil
}

func (o *Manifest) GetImageManifest(name string) (*v1alpha3.OCISchema, error) {
	if o.FailImageManifest {
		return &v1alpha3.OCISchema{}, fmt.Errorf("forced error image index")
	}

	return &v1alpha3.OCISchema{
		SchemaVersion: 2,
		Manifests: []v1alpha3.OCIManifest{
			{
				MediaType: "application/vnd.oci.image.manifest.v1+json",
				Digest:    "sha256:3ef0b0141abd1548f60c4f3b23ecfc415142b0e842215f38e98610a3b2e52419",
				Size:      567,
			},
		},
		Config: v1alpha3.OCIManifest{
			MediaType: "application/vnd.oci.image.manifest.v1+json",
			Digest:    "sha256:3ef0b0141abd1548f60c4f3b23ecfc415142b0e842215f38e98610a3b2e52419",
			Size:      567,
		},
	}, nil
}

func (o *Manifest) GetRelatedImagesFromCatalog(filePath, label string) (map[string][]v1alpha3.RelatedImage, error) {
	relatedImages := make(map[string][]v1alpha3.RelatedImage)
	relatedImages["abc"] = []v1alpha3.RelatedImage{
		{Name: "testA", Image: "sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
		{Name: "testB", Image: "sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
	}
	return relatedImages, nil
}

func (o *Manifest) ExtractLayersOCI(filePath, toPath, label string, oci *v1alpha3.OCISchema) error {
	if o.FailExtract {
		return fmt.Errorf("forced extract oci fail")
	}
	return nil
}

func (o *Cincinnati) GetReleaseReferenceImages(ctx context.Context) []v1alpha3.CopyImageSchema {
	var res []v1alpha3.CopyImageSchema
	res = append(res, v1alpha3.CopyImageSchema{Source: "test", Destination: "test"})
	return res
}

func (o *Cincinnati) NewOCPClient(uuid uuid.UUID) (Client, error) {
	if o.Fail {
		return o.Client, fmt.Errorf("forced cincinnati client error")
	}
	return o.Client, nil
}

func (o *Cincinnati) NewOKDClient(uuid uuid.UUID) (Client, error) {
	return o.Client, nil
}

func (o *Cincinnati) GenerateReleaseSignatures(context.Context, []v1alpha3.RelatedImage) {
	fmt.Println("test release signature")
}
