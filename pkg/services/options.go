package services

import (
	"context"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/klog/v2"
)

type RootOptions struct {
	genericclioptions.IOStreams
	Dir            string
	LogLevel       int
	logfileCleanup func()
}

func (o *RootOptions) BindFlags(fs *pflag.FlagSet) {
	fs.StringVarP(&o.Dir, "dir", "d", "oc-mirror-workspace", "Assets directory")
	fs.IntVarP(&o.LogLevel, "verbose", "v", o.LogLevel, "Number for the log level verbosity (valid 1-9, default is 0)")
	if err := fs.MarkHidden("dir"); err != nil {
		klog.Fatal(err.Error())
	}
}

func (o *RootOptions) LogfilePreRun(cmd *cobra.Command, _ []string) {
	var fsv2 flag.FlagSet
	// Configure klog flags
	klog.InitFlags(&fsv2)
	checkErr(fsv2.Set("stderrthreshold", "4"))
	checkErr(fsv2.Set("skip_headers", "true"))
	checkErr(fsv2.Set("logtostderr", "false"))
	checkErr(fsv2.Set("alsologtostderr", "false"))
	checkErr(fsv2.Set("v", fmt.Sprintf("%d", o.LogLevel)))

	logFile, err := os.OpenFile(".oc-mirror.log", os.O_CREATE|os.O_APPEND|os.O_RDWR, 0600)
	if err != nil {
		klog.Fatal(err)
	}

	klog.SetOutput(io.MultiWriter(o.IOStreams.Out, logFile))

	// Setup logrus for use with operator-registry
	logrus.SetOutput(ioutil.Discard)

	var logrusLevel logrus.Level
	switch o.LogLevel {
	case 0:
		logrusLevel = logrus.InfoLevel
	case 1:
		logrusLevel = logrus.DebugLevel
	case 2:
		logrusLevel = logrus.DebugLevel
	default:
		logrusLevel = logrus.TraceLevel
	}

	logrus.SetLevel(logrusLevel)
	//logrus.AddHook(newFileHookWithNewlineTruncate(o.IOStreams.ErrOut, logrusLevel, &logrus.TextFormatter{
	//	DisableTimestamp:       false,
	//		DisableLevelTruncation: true,
	//		DisableQuote:           true,
	//	}))
	//	logrusCleanup := setupFileHook(logFile)

	// Add to root IOStream options
	o.IOStreams = genericclioptions.IOStreams{
		In:     o.IOStreams.In,
		Out:    io.MultiWriter(o.IOStreams.Out, logFile),
		ErrOut: io.MultiWriter(o.IOStreams.ErrOut, logFile),
	}

	o.logfileCleanup = func() {
		klog.Flush()
		//		logrusCleanup()
		checkErr(logFile.Close())
	}

}

func (o *RootOptions) LogfilePostRun(*cobra.Command, []string) {
	if o.logfileCleanup != nil {
		o.logfileCleanup()
	}
}

func checkErr(err error) {
	if err != nil {
		klog.Fatal(err)
	}
}

type MirrorOptions struct {
	*RootOptions
	OutputDir                  string
	ConfigPath                 string
	SkipImagePin               bool
	ManifestsOnly              bool
	From                       string
	ToMirror                   string
	UserNamespace              string
	DryRun                     bool
	SourceSkipTLS              bool
	DestSkipTLS                bool
	SourcePlainHTTP            bool
	DestPlainHTTP              bool
	SkipVerification           bool
	SkipCleanup                bool
	SkipMissing                bool
	SkipMetadataCheck          bool
	ContinueOnError            bool
	IgnoreHistory              bool
	MaxPerRegistry             int
	UseOCIFeature              bool
	OCIRegistriesConfig        string
	OCIInsecureSignaturePolicy bool
	// cancelCh is a channel listening for command cancellations
	cancelCh         <-chan struct{}
	once             sync.Once
	continuedOnError bool
	loglevel         string
	//remoteRegFuncs   RemoteRegFuncs
}

func (o *MirrorOptions) BindFlags(fs *pflag.FlagSet) {
	fs.StringVarP(&o.ConfigPath, "config", "c", o.ConfigPath, "Path to imageset configuration file")
	fs.StringVarP(&o.loglevel, "loglevel", "l", o.loglevel, "Log level one of (info, debug, trace, error)")
	fs.BoolVar(&o.DryRun, "dry-run", o.DryRun, "Print actions without mirroring images")
	fs.BoolVar(&o.SkipVerification, "skip-verification", o.SkipVerification, "Skip verifying the integrity of the retrieved content."+
		"This is not recommended, but may be necessary when importing images from older image registries."+
		"Only bypass verification if the registry is known to be trustworthy.")
	fs.BoolVar(&o.ContinueOnError, "continue-on-error", o.ContinueOnError, "If an error occurs, keep going "+
		"and attempt to complete operations if possible")
	fs.BoolVar(&o.SkipMissing, "skip-missing", o.SkipMissing, "If an input image is not found, skip them. "+
		"404/NotFound errors encountered while pulling images explicitly specified in the config "+
		"will not be skipped")
}

func (o *MirrorOptions) init() {
	o.cancelCh = makeCancelCh(syscall.SIGINT, syscall.SIGTERM)
}

// CancelContext will return a cancellable context and listen for
// cancellation signals
func (o *MirrorOptions) CancelContext(parent context.Context) (context.Context, context.CancelFunc) {
	o.once.Do(o.init)
	ctx, cancel := context.WithCancel(parent)
	go func() {
		select {
		case <-o.cancelCh:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}

// makeCancelCh creates an interrupt listener for os signals
// and will send a message on a returned channel
func makeCancelCh(signals ...os.Signal) <-chan struct{} {
	resultCh := make(chan struct{})
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, signals...)
	go func() {
		for {
			<-signalCh
			resultCh <- struct{}{}
		}
	}()
	return resultCh
}
