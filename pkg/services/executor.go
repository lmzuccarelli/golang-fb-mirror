package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/lmzuccarelli/golang-oci-mirror/pkg/api/v1alpha2"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/config"
	clog "github.com/lmzuccarelli/golang-oci-mirror/pkg/log"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/manifest"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/mirror"
	"github.com/spf13/cobra"
)

var (
	mirrorlongDesc = templates.LongDesc(
		` 
		Create and publish user-configured mirrors with a declarative configuration input.
		used for authenticating to the registries. The podman location for credentials is also supported as a secondary location.

		1. Destination prefix is docker:// - The current working directory will be used.
		2. Destination prefix is oci:// - The destination directory specified will be used.


		TODO:

		`,
	)
	mirrorExamples = templates.Examples(
		`
		# Mirror to a directory
		oc-mirror --config mirror-config.yaml oci:mirror

		TODO:
		`,
	)
)

// Executor
type Executor struct {
	Log      clog.PluggableLoggerInterface
	Mirror   mirror.MirrorInterface
	Manifest manifest.ManifestInterface
	Config   v1alpha2.ImageSetConfiguration
	Opts     mirror.CopyOptions
}

// NewMirrorCmd - cobra entry point
func NewMirrorCmd() *cobra.Command {
	o := MirrorOptions{}

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
			kcmdutil.CheckErr(o.Validate(args[0]))
			kcmdutil.CheckErr(o.Run(cmd, args))
		},
	}

	o.BindFlags(cmd.Flags())
	return cmd
}

// Run - start the mirror functionality
func (o *MirrorOptions) Run(cmd *cobra.Command, args []string) error {

	// setup pluggable logger
	// feel free to plugin you own logger
	// just use the PluggableLoggerInterface
	// in the file pkg/log.go
	log := clog.New("debug")

	// read the ImageSetConfiguration
	cfg, err := config.ReadConfig(o.ConfigPath)
	if err != nil {
		log.Error("imagesetconfig %v ", err)
	}
	log.Debug("imagesetconfig : %v", cfg)

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
		Destination:         args[0],
		Dev:                 false,
	}

	// logic to check mode
	if strings.Contains(args[0], ociProtocol) {
		opts.Mode = mirrorToDisk
	} else if strings.Contains(args[0], dockerProtocol) {
		opts.Mode = diskToMirror
	}

	log.Info("mode %s ", opts.Mode)

	executor := &Executor{
		Log:      log,
		Mirror:   mirror.New(),
		Manifest: manifest.New(log),
		Config:   cfg,
		Opts:     opts,
	}

	// ensure working dir exists
	err = os.MkdirAll("working-dir", 0755)
	if err != nil {
		log.Error(" %v ", err)
		os.Exit(1)
	}

	// do releases
	if len(cfg.Mirror.Platform.Channels) > 10 {
		err = executor.ExecuteRelease(cmd.Context())
		if err != nil {
			log.Error(fmt.Sprintf("executing release copy %v ", err))
			os.Exit(1)
		}
	}

	// do operators
	if len(cfg.Mirror.Operators) > 0 {
		err = executor.ExecuteOperators(cmd.Context())
		if err != nil {
			log.Error(fmt.Sprintf("executing operator copy %v ", err))
			os.Exit(1)
		}
	}
	return nil
}

// Validate - cobra validation
func (o *MirrorOptions) Validate(dest string) error {
	if strings.Contains(dest, "oci:") || strings.Contains(dest, "docker://") {
		return nil
	}
	return fmt.Errorf("destination protocol must be either oci: or docker://")
}
