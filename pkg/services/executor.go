package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/lmzuccarelli/golang-oci-mirror/pkg/config"
	"github.com/microlib/simple"
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

func (o *MirrorOptions) Run(cmd *cobra.Command, args []string) error {
	logger := &simple.Logger{Level: "debug"}
	cfg, err := config.ReadConfig(o.ConfigPath)
	if err != nil {
		logger.Error(fmt.Sprintf("imagesetconfig %v ", err))
	}
	logger.Debug(fmt.Sprintf("imagesetconfig : %v", cfg))

	global := &GlobalOptions{Debug: true, TlsVerify: false, InsecurePolicy: true, Destination: args[0]}
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
	}

	// ensure working dir exists
	err = os.MkdirAll("working-dir", 0755)
	if err != nil {
		logger.Error(fmt.Sprintf(" %v ", err))
		os.Exit(1)
	}

	// do releases
	if len(cfg.Mirror.Platform.Channels) > 10 {
		err = ExecuteRelease(logger, cfg, opts)
		if err != nil {
			logger.Error(fmt.Sprintf("executing release copy %v ", err))
			os.Exit(1)
		}
	}

	// do operators
	if len(cfg.Mirror.Operators) > 0 {
		err = ExecuteOperators(logger, cfg, opts)
		if err != nil {
			logger.Error(fmt.Sprintf("executing release copy %v ", err))
			os.Exit(1)
		}

	}
	return nil
}

func (o *MirrorOptions) Validate(dest string) error {
	if strings.Contains(dest, "oci:") || strings.Contains(dest, "docker://") {
		return nil
	}
	return fmt.Errorf("destination protocol must be either oci: or docker://")
}
