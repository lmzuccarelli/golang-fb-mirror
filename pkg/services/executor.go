package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

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
		Short:   "Manage mirrors per user configuration",
		Long:    mirrorlongDesc,
		Example: mirrorExamples,
		//PersistentPreRun:  o.LogfilePreRun,
		//PersistentPostRun: o.LogfilePostRun,
		Args:          cobra.MinimumNArgs(1),
		SilenceErrors: false,
		SilenceUsage:  false,
		Run: func(cmd *cobra.Command, args []string) {
			//kcmdutil.CheckErr(o.Complete(cmd, args))
			kcmdutil.CheckErr(ex.Validate(args))
			kcmdutil.CheckErr(ex.Run(cmd, args))
		},
	}

	o.BindFlags(cmd.Flags())
	opts.Global.ConfigPath = o.ConfigPath

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

	// read the ImageSetConfiguration
	cfg, err := config.ReadConfig(o.Opts.Global.ConfigPath)
	if err != nil {
		o.Log.Error("imagesetconfig %v ", err)
	}
	o.Log.Debug("imagesetconfig : %v", cfg)

	o.Config = cfg
	//o.Operator = operator.New(o.Log, o.Config, o.Opts, o.Mirror, o.Manifest)
	//o.Mirror = mirror.New()
	//o.Batch = batch.New(o.Log, o.Config, o.Opts, o.Mirror, o.Manifest)

	// ensure working dir exists
	err = os.MkdirAll("working-dir", 0755)
	if err != nil {
		o.Log.Error(" %v ", err)
		return err
	}

	var allRelatedImages []v1alpha3.RelatedImage

	// do releases
	if len(cfg.Mirror.Platform.Channels) > 0 {
		// add these in the BatchWorker
		// src := dockerProtocol + release.Spec.Tags[index].From.Name
		// dest := strings.Split(release.Spec.Tags[index].From.Name, ":")[1]
		allRelatedImages, err = o.Release.ReleaseImageCollector(cmd.Context())
		if err != nil {
			return err
		}
		o.Log.Info("total release images to copy %d ", len(allRelatedImages))
		//call the batch executioner
	}

	// do operators
	if len(cfg.Mirror.Operators) > 0 {
		allRelatedImages, err = o.Operator.OperatorImageCollector(cmd.Context())
		if err != nil {
			return err
		}
		o.Log.Info("total operator images to copy %d ", len(allRelatedImages))
	}

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
