package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"k8s.io/kubectl/pkg/util/templates"

	"github.com/google/uuid"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/additional"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/api/v1alpha2"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/batch"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/config"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/diff"
	clog "github.com/lmzuccarelli/golang-oci-mirror/pkg/log"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/manifest"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/mirror"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/operator"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/release"
	"github.com/spf13/cobra"
)

const (
	dockerProtocol  string = "docker://"
	ociProtocol     string = "oci://"
	diskToMirror    string = "diskToMirror"
	mirrorToDisk    string = "mirrorToDisk"
	releaseImageDir string = "release-images"
)

var (
	mirrorlongDesc = templates.LongDesc(
		` 
		Create and publish user-configured mirrors with a declarative configuration input.
		used for authenticating to the registries. 

		The podman location for credentials is also supported as a secondary location.

		1. Destination prefix is docker:// - The current working directory will be used.
		2. Destination prefix is oci:// - The destination directory specified will be used.

		`,
	)
	mirrorExamples = templates.Examples(
		`
		# Mirror to a directory
		oc-mirror oci:mirror --config mirror-config.yaml
		`,
	)
)

const (
	logsDir                 string = "logs/"
	workingDir              string = "working-dir/"
	additionalImages        string = "additional-images"
	releaseImageExtractDir  string = "hold-release"
	operatorImageExtractDir string = "hold-operator"
)

type ExecutorSchema struct {
	Log              clog.PluggableLoggerInterface
	Config           v1alpha2.ImageSetConfiguration
	MetaData         diff.SequenceSchema
	Opts             mirror.CopyOptions
	Operator         operator.CollectorInterface
	Release          release.CollectorInterface
	AdditionalImages additional.CollectorInterface
	Mirror           mirror.MirrorInterface
	Manifest         manifest.ManifestInterface
	Batch            batch.BatchInterface
	Diff             diff.DiffInterface
}

// NewMirrorCmd - cobra entry point
func NewMirrorCmd(log clog.PluggableLoggerInterface) *cobra.Command {

	global := &mirror.GlobalOptions{
		TlsVerify:      false,
		InsecurePolicy: true,
	}

	flagSharedOpts, sharedOpts := mirror.SharedImageFlags()
	flagDepTLS, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	flagSrcOpts, srcOpts := mirror.ImageFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	flagDestOpts, destOpts := mirror.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	flagRetryOpts, retryOpts := mirror.RetryFlags()

	opts := mirror.CopyOptions{
		Global:              global,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOpts,
		DestImage:           destOpts,
		RetryOpts:           retryOpts,
		Dev:                 false,
	}

	ex := &ExecutorSchema{
		Log:  log,
		Opts: opts,
	}

	cmd := &cobra.Command{
		Use:           fmt.Sprintf("%v <destination type>:<destination location>", filepath.Base(os.Args[0])),
		Version:       "v0.0.1",
		Short:         "Manage mirrors per user configuration",
		Long:          mirrorlongDesc,
		Example:       mirrorExamples,
		Args:          cobra.MinimumNArgs(1),
		SilenceErrors: false,
		SilenceUsage:  false,
		Run: func(cmd *cobra.Command, args []string) {
			err := ex.Validate(args)
			if err != nil {
				log.Error("%v ", err)
				os.Exit(1)
			}

			ex.Complete(args)

			err = ex.Run(cmd, args)
			if err != nil {
				log.Error("%v ", err)
				os.Exit(1)
			}
		},
	}

	cmd.PersistentFlags().StringVar(&opts.Global.ConfigPath, "config", "", "Path to imageset configuration file")
	cmd.Flags().StringVar(&opts.Global.LogLevel, "loglevel", "info", "Log level one of (info, debug, trace, error)")
	cmd.Flags().StringVar(&opts.Global.Dir, "dir", "working-dir", "Assets directory")
	cmd.Flags().StringVar(&opts.Global.From, "from", "", "directory used when doing the oci: (mirrorToDisk) mode")
	cmd.Flags().BoolVarP(&opts.Global.Quiet, "quiet", "q", false, "enable detailed logging when copying images")
	cmd.Flags().BoolVarP(&opts.Global.Force, "force", "f", false, "force the copy and mirror functionality")
	cmd.Flags().AddFlagSet(&flagSharedOpts)
	cmd.Flags().AddFlagSet(&flagRetryOpts)
	cmd.Flags().AddFlagSet(&flagDepTLS)
	cmd.Flags().AddFlagSet(&flagSrcOpts)
	cmd.Flags().AddFlagSet(&flagDestOpts)
	return cmd
}

// Run - start the mirror functionality
func (o *ExecutorSchema) Run(cmd *cobra.Command, args []string) error {

	// clean up logs directory
	os.RemoveAll(logsDir)
	// ensure working dir exists
	err := os.MkdirAll(workingDir, 0755)
	if err != nil {
		o.Log.Error(" %v ", err)
		return err
	}

	// create logs directory
	err = os.MkdirAll(logsDir, 0755)
	if err != nil {
		o.Log.Error(" %v ", err)
		return err
	}

	// create release cache dir
	o.Log.Trace("creating release cache directory %s ", o.Opts.Global.Dir+"/"+releaseImageExtractDir)
	err = os.MkdirAll(o.Opts.Global.Dir+"/"+releaseImageExtractDir, 0755)
	if err != nil {
		o.Log.Error(" %v ", err)
		return err
	}

	// create operator cache dir
	o.Log.Trace("creating operator cache directory %s ", o.Opts.Global.Dir+"/"+operatorImageExtractDir)
	err = os.MkdirAll(o.Opts.Global.Dir+"/"+operatorImageExtractDir, 0755)
	if err != nil {
		o.Log.Error(" %v ", err)
		return err
	}

	var allRelatedImages []string

	// check if we need to copy or mirror
	// check if there is a change if so then continue as normal else
	// report that there is nothing to do (all up to date)
	_, prevCfg, err := o.Diff.GetAllMetadata(o.Opts.Global.Dir)
	if err != nil {
		o.Log.Error("%v", err)
	}
	res, err := o.Diff.CheckDiff(prevCfg)
	if err != nil {
		o.Log.Error("%v", err)
	}

	// check if there has been a change in the imagesetconfig
	// between a copy and mirror - it should not be changed
	if o.Opts.Mode == diskToMirror && res && !o.Opts.Global.Force {
		return fmt.Errorf("imagesetconfig should not be changed (from copy)")
	}

	if !res && !o.Opts.Global.Force {
		o.Log.Info("no change detected for copy and mirror")
		return nil
	}

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

	// do additionalImages
	imgs, err = o.AdditionalImages.AdditionalImagesCollector(cmd.Context())
	if err != nil {
		return err
	}
	o.Log.Info("total additional images to copy %d ", len(imgs))
	allRelatedImages = mergeImages(allRelatedImages, imgs)

	//call the batch worker
	err = o.Batch.Worker(cmd.Context(), allRelatedImages, o.Opts)
	if err != nil {
		return err
	}

	// only execute if mode is diskToMirror
	err = o.Diff.DeleteImages(cmd.Context())
	if err != nil {
		o.Log.Error("%v", err)
	}
	return nil
}

// Complete - do the final setup of modules
func (o *ExecutorSchema) Complete(args []string) {
	// override log level
	o.Log.Level(o.Opts.Global.LogLevel)
	o.Log.Debug("imagesetconfig file %s ", o.Opts.Global.ConfigPath)
	// read the ImageSetConfiguration
	cfg, err := config.ReadConfig(o.Opts.Global.ConfigPath)
	if err != nil {
		o.Log.Error("imagesetconfig %v ", err)
	}
	o.Log.Trace("imagesetconfig : %v ", cfg)

	// update all dependant modules
	mc := mirror.NewMirrorCopy()
	md := mirror.NewMirrorDelete()
	o.Manifest = manifest.New(o.Log)
	o.Mirror = mirror.New(mc, md)
	o.Config = cfg
	o.Batch = batch.New(o.Log, o.Mirror, o.Manifest)

	// logic to check mode
	var dest string
	if strings.Contains(args[0], ociProtocol) {
		o.Opts.Mode = mirrorToDisk
		dest = workingDir + strings.Split(args[0], ociProtocol)[1]
		o.Log.Debug("destination %s ", dest)
	} else if strings.Contains(args[0], dockerProtocol) {
		o.Opts.Mode = diskToMirror
		dest = o.Opts.Global.From
		o.Log.Debug("destination %s ", dest)
	}
	o.Opts.Destination = args[0]
	o.Opts.Global.Dir = dest
	o.Log.Info("mode %s ", o.Opts.Mode)

	client, _ := release.NewOCPClient(uuid.New())
	cn := release.NewCincinnati(o.Log, &o.Config, &o.Opts, client, false)
	o.Release = release.New(o.Log, o.Config, o.Opts, o.Mirror, o.Manifest, cn)
	o.Operator = operator.New(o.Log, o.Config, o.Opts, o.Mirror, o.Manifest)
	o.AdditionalImages = additional.New(o.Log, o.Config, o.Opts, o.Mirror, o.Manifest)
	o.Diff = diff.New(o.Log, o.Config, o.Opts, o.Mirror)

	metadata, _, err := o.Diff.GetAllMetadata(dest)
	if err != nil {
		// if no previous imagesetconfig was found create new one
		item := []diff.Item{
			{
				Value:       0,
				Current:     true,
				Timestamp:   time.Now().Unix(),
				Destination: dest,
			},
		}
		seq := diff.Sequence{Item: item}
		metadata = diff.SequenceSchema{Title: "golang-oci-mirror", Owner: "CFE-EAMA", Sequence: seq}
		o.Log.Info("added new metadata %v ", metadata)
		err := o.Diff.WriteMetadata(o.Opts.Global.ConfigPath, dest, metadata, o.Config)
		if err != nil {
			o.Log.Error("%v", err)
		}
	}
	o.MetaData = metadata
	o.Log.Info("metadata %v ", metadata)
}

// Validate - cobra validation
func (o *ExecutorSchema) Validate(dest []string) error {
	if len(o.Opts.Global.ConfigPath) == 0 && strings.Contains(dest[0], ociProtocol) {
		return fmt.Errorf("use the --config flag when using oci: protocol")
	}
	if len(o.Opts.Global.From) == 0 && strings.Contains(dest[0], dockerProtocol) {
		return fmt.Errorf("use the --from flag when using docker: protocol")
	}
	if strings.Contains(dest[0], ociProtocol) || strings.Contains(dest[0], dockerProtocol) {
		return nil
	} else {
		return fmt.Errorf("destination must have either oci:// or docker:// protocol prefixes")
	}
}

// mergeImages - simple function to append releated images
// nolint
func mergeImages(base, in []string) []string {
	for _, img := range in {
		base = append(base, img)
	}
	return base
}
