package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/google/uuid"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/api/v1alpha2"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/api/v1alpha3"
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
	log.Info("executor schema %v ", ex)

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
			kcmdutil.CheckErr(ex.Validate(args))
			kcmdutil.CheckErr(ex.Run(cmd, args))
		},
	}

	cmd.PersistentFlags().StringVar(&o.ConfigPath, "config", "isc.yaml", "Path to imageset configuration file")
	opts.Global.ConfigPath = o.ConfigPath
	// set t odev mode
	opts.Dev = true
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
	ex.Operator = operator.New(log, cfg, opts, ex.Mirror, ex.Manifest)
	ex.Batch = batch.New(log, ex.Mirror, ex.Manifest)
	// TODO: error handling
	client, _ := release.NewOCPClient(uuid.New())
	cn := release.NewCincinnati(&cfg, &opts, client, false)
	ex.Release = release.New(log, cfg, opts, ex.Mirror, ex.Manifest, cn)
	ex.Operator = operator.New(log, cfg, opts, ex.Mirror, ex.Manifest)
	return cmd
}

// Run - start the mirror functionality
func (o *ExecutorSchema) Run(cmd *cobra.Command, args []string) error {

	// logic to check mode
	if strings.Contains(args[0], ociProtocol) {
		o.Opts.Mode = mirrorToDisk
	} else if strings.Contains(args[0], dockerProtocol) {
		o.Opts.Mode = diskToMirror
	}

	o.Log.Info("mode %s ", o.Opts.Mode)
	o.Opts.Destination = args[0]

	// ensure working dir exists
	err := os.MkdirAll("working-dir", 0755)
	if err != nil {
		o.Log.Error(" %v ", err)
		return err
	}

	var allRelatedImages []v1alpha3.RelatedImage

	// do releases
	if len(o.Config.Mirror.Platform.Channels) > 0 {
		// add these in the BatchWorker
		// src := dockerProtocol + release.Spec.Tags[index].From.Name
		// dest := strings.Split(release.Spec.Tags[index].From.Name, ":")[1]
		ri, err := o.Release.ReleaseImageCollector(cmd.Context())
		if err != nil {
			return err
		}
		o.Log.Info("total release images to copy %d ", len(ri))
		o.Opts.ImageType = "release"
		allRelatedImages = mergeImages(allRelatedImages, ri)
	}

	// do operators
	if len(o.Config.Mirror.Operators) > 0 {
		ri, err := o.Operator.OperatorImageCollector(cmd.Context())
		if err != nil {
			return err
		}
		o.Log.Info("total operator images to copy %d ", len(ri))
		o.Opts.ImageType = "operator"
		allRelatedImages = mergeImages(allRelatedImages, ri)
	}

	//call the batch executioner
	err = o.Batch.Worker(cmd.Context(), allRelatedImages, o.Opts)
	if err != nil {
		return err
	}

	return nil
}

// Validate - cobra validation
func (o *ExecutorSchema) Validate(dest []string) error {
	if strings.Contains(dest[0], "oci:") || strings.Contains(dest[0], "docker://") {
		return nil
	}
	return fmt.Errorf("destination protocol must be either oci: or docker://")
}

//nolint
func mergeImages(base, in []v1alpha3.RelatedImage) []v1alpha3.RelatedImage {
	for _, img := range in {
		base = append(base, img)
	}
	return base
}
