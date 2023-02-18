package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/kubectl/pkg/util/templates"

	"github.com/google/uuid"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/api/v1alpha2"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/batch"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/config"
	clog "github.com/lmzuccarelli/golang-oci-mirror/pkg/log"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/manifest"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/mirror"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/operator"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/release"
	"github.com/spf13/cobra"
)

const (
	dockerProtocol  string = "docker://"
	ociProtocol     string = "oci:"
	diskToMirror    string = "diskToMirror"
	mirrorToDisk    string = "mirrorToDisk"
	releaseImageDir string = "release-images"
)

var (
	mirrorlongDesc = templates.LongDesc(
		` 
		Create and publish user-configured mirrors with a declarative configuration input.
		used for authenticating to the registries. The podman location for credentials is also supported as a secondary location.

		1. Destination prefix is docker:// - The current working directory will be used.
		2. Destination prefix is oci:// - The destination directory specified will be used.



		`,
	)
	mirrorExamples = templates.Examples(
		`
		# Mirror to a directory
		oc-mirror --config mirror-config.yaml oci:mirror

		`,
	)
)

type ExecutorSchema struct {
	Log        clog.PluggableLoggerInterface
	Config     v1alpha2.ImageSetConfiguration
	Opts       mirror.CopyOptions
	Operator   operator.CollectorInterface
	Release    release.CollectorInterface
	Mirror     mirror.MirrorInterface
	Manifest   manifest.ManifestInterface
	Batch      batch.BatchInterface
	MirrorOpts MirrorOptions
}

// NewMirrorCmd - cobra entry point
func NewMirrorCmd() *cobra.Command {

	o := MirrorOptions{}

	global := &mirror.GlobalOptions{
		Debug:          true,
		TlsVerify:      false,
		InsecurePolicy: true,
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
	}

	// setup pluggable logger
	// feel free to plugin you own logger
	// just use the PluggableLoggerInterface
	// in the file pkg/log.go
	log := clog.New("debug")
	ex := &ExecutorSchema{
		Log:  log,
		Opts: opts,
	}
	log.Debug("executor schema %v ", ex)

	cmd := &cobra.Command{
		Use: fmt.Sprintf(
			"%s <destination type>:<destination location>",
			filepath.Base(os.Args[0]),
		),
		Short:         "Manage mirrors per user configuration",
		Long:          mirrorlongDesc,
		Example:       mirrorExamples,
		Args:          cobra.MinimumNArgs(1),
		SilenceErrors: false,
		SilenceUsage:  false,
		Run: func(cmd *cobra.Command, args []string) {
			ex.Complete(args)
			err := ex.Validate(args)
			if err != nil {
				log.Error("%v ", err)
				os.Exit(1)
			}
			err = ex.Run(cmd, args)
			if err != nil {
				log.Error("%v ", err)
				os.Exit(1)
			}
		},
	}

	cmd.PersistentFlags().StringVar(&o.ConfigPath, "config", "isc.yaml", "Path to imageset configuration file")
	opts.Global.ConfigPath = o.ConfigPath
	log.Debug("imagesetconfig file %s ", o.ConfigPath)
	// read the ImageSetConfiguration
	cfg, err := config.ReadConfig(opts.Global.ConfigPath)
	if err != nil {
		log.Error("imagesetconfig %v ", err)
	}
	log.Debug("imagesetconfig : %v ", cfg)

	// update all dependant modules
	mc := mirror.NewMirrorCopy()
	ex.Manifest = manifest.New(log)
	ex.Mirror = mirror.New(mc)
	ex.Config = cfg
	ex.Batch = batch.New(log, ex.Mirror, ex.Manifest)

	return cmd
}

// Run - start the mirror functionality
func (o *ExecutorSchema) Run(cmd *cobra.Command, args []string) error {

	// clean up logs directory
	os.RemoveAll("logs/")
	// ensure working dir exists
	err := os.MkdirAll("working-dir", 0755)
	if err != nil {
		o.Log.Error(" %v ", err)
		return err
	}
	// create logs directory
	err = os.MkdirAll("logs", 0755)
	if err != nil {
		o.Log.Error(" %v ", err)
		return err
	}

	var allRelatedImages []string

	// do releases
	imgs, err := o.Release.ReleaseImageCollector(cmd.Context())
	if err != nil {
		return err
	}
	o.Log.Info("total release images to copy %d ", len(imgs))
	o.Opts.ImageType = "release"
	allRelatedImages = mergeImages(allRelatedImages, imgs)

	// do operators
	imgs, err = o.Operator.OperatorImageCollector(cmd.Context())
	if err != nil {
		return err
	}
	o.Log.Info("total operator images to copy %d ", len(imgs))
	o.Opts.ImageType = "operator"
	allRelatedImages = mergeImages(allRelatedImages, imgs)

	//call the batch worker
	err = o.Batch.Worker(cmd.Context(), allRelatedImages, o.Opts)
	if err != nil {
		return err
	}

	return nil
}

// Complete - do the final setup of modules
func (o *ExecutorSchema) Complete(args []string) {
	// logic to check mode
	if strings.Contains(args[0], ociProtocol) {
		o.Opts.Mode = mirrorToDisk
	} else if strings.Contains(args[0], dockerProtocol) {
		o.Opts.Mode = diskToMirror
	}
	o.Opts.Dev = true
	o.Log.Info("mode %s ", o.Opts.Mode)
	o.Opts.Destination = args[0]
	o.Opts.Global.From = "working-dir/test-lmz"
	client, _ := release.NewOCPClient(uuid.New())
	cn := release.NewCincinnati(o.Log, &o.Config, &o.Opts, client, false)
	o.Release = release.New(o.Log, o.Config, o.Opts, o.Mirror, o.Manifest, cn)
	o.Operator = operator.New(o.Log, o.Config, o.Opts, o.Mirror, o.Manifest)
}

// Validate - cobra validation
func (o *ExecutorSchema) Validate(dest []string) error {
	if strings.Contains(dest[0], "oci:") || strings.Contains(dest[0], "docker://") {
		return nil
	}
	return fmt.Errorf("destination protocol must be either oci: or docker://")
}

// mergeImages - simple function to append releated images
//nolint
func mergeImages(base, in []string) []string {
	for _, img := range in {
		base = append(base, img)
	}
	return base
}
