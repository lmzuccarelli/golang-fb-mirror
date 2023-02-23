package additional

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lmzuccarelli/golang-oci-mirror/pkg/api/v1alpha2"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/api/v1alpha3"
	clog "github.com/lmzuccarelli/golang-oci-mirror/pkg/log"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/manifest"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/mirror"
)

const (
	indexJson               string = "index.json"
	operatorImageExtractDir string = "hold-operator"
	workingDir              string = "working-dir/"
	dockerProtocol          string = "docker://"
	ociProtocol             string = "oci://"
	ociProtocolTrimmed      string = "oci:"
	additionalImagesDir     string = "additional-images"
	blobsDir                string = "/blobs/sha256/"
	diskToMirror            string = "diskToMirror"
	mirrorToDisk            string = "mirrorToDisk"
	errMsg                  string = "[AdditionalImagesCollector] %v "
	logsFile                string = "logs/additional-images.log"
)

type CollectorInterface interface {
	AdditionalImagesCollector(ctx context.Context) ([]string, error)
}

func New(log clog.PluggableLoggerInterface,
	config v1alpha2.ImageSetConfiguration,
	opts mirror.CopyOptions,
	mirror mirror.MirrorInterface,
	manifest manifest.ManifestInterface,
) CollectorInterface {
	return &Collector{Log: log, Config: config, Opts: opts, Mirror: mirror, Manifest: manifest}
}

type Collector struct {
	Log      clog.PluggableLoggerInterface
	Mirror   mirror.MirrorInterface
	Manifest manifest.ManifestInterface
	Config   v1alpha2.ImageSetConfiguration
	Opts     mirror.CopyOptions
}

// AdditionalImagesCollector - this looks into the additional images field
// taking into account the mode we are in (mirrorToDisk, diskToMirror)
// the image is downloaded in oci format
func (o *Collector) AdditionalImagesCollector(ctx context.Context) ([]string, error) {

	var allImages []string
	if !strings.Contains(o.Opts.Destination, ociProtocol) && !strings.Contains(o.Opts.Destination, dockerProtocol) {
		return []string{}, fmt.Errorf(errMsg, "destination must use oci:// or docker:// prefix")
	}

	if o.Opts.Mode == mirrorToDisk {
		for _, img := range o.Config.ImageSetConfigurationSpec.Mirror.AdditionalImages {
			irs, err := customImageParser(img.Name)
			o.Log.Debug("%v", irs)
			if err != nil {
				return []string{}, nil
			}
			src := dockerProtocol + img.Name
			dest := ociProtocolTrimmed + o.Opts.Global.Dir + "/" + additionalImagesDir + "/" + irs.Component
			o.Log.Trace("source %s", src)
			o.Log.Trace("destination %s", dest)
			allImages = append(allImages, src+"*"+dest)
		}
	}

	if o.Opts.Mode == diskToMirror {
		regex, e := regexp.Compile(indexJson)
		if e != nil {
			o.Log.Error("%v", e)
		}
		e = filepath.Walk(workingDir+"/"+o.Opts.Global.From+"/"+additionalImagesDir, func(path string, info os.FileInfo, err error) error {
			if err == nil && regex.MatchString(info.Name()) {
				ns := strings.Split(filepath.Dir(path), additionalImagesDir)
				if len(ns) == 0 {
					return fmt.Errorf(errMsg+"%s", "no directory found for additional-images ", path)
				} else {
					name := strings.Split(ns[1], "/")
					if len(name) != 2 {
						return fmt.Errorf(errMsg+" %s ", "additional images name and related compents are incorrect", name)
					}
					src := strings.Trim(ociProtocol, "/") + ns[0] + additionalImagesDir + "/" + name[1]
					dest := o.Opts.Destination + "/" + name[1]
					allImages = append(allImages, src+"*"+dest)
				}
			}
			return nil
		})
		if e != nil {
			return []string{}, e
		}
	}
	return allImages, nil
}

// customImageParser - simple image string parser
func customImageParser(image string) (*v1alpha3.ImageRefSchema, error) {
	var irs *v1alpha3.ImageRefSchema
	var component string
	parts := strings.Split(image, "/")
	if len(parts) < 3 {
		return irs, fmt.Errorf("[customImageParser] image url seems to be wrong %s ", image)
	}
	component = parts[2]
	if strings.Contains(parts[2], "@") {
		component = strings.Split(parts[2], "@")[0]
	}
	if strings.Contains(parts[2], ":") {
		component = strings.Split(parts[2], ":")[0]
	}
	irs = &v1alpha3.ImageRefSchema{Repository: parts[0], Namespace: parts[1], Component: component}
	return irs, nil
}
