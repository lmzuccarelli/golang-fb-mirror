package release

import (
	"bufio"
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
	indexJson                   string = "index.json"
	operatorImageExtractDir     string = "hold-operator"
	workingDir                  string = "working-dir/"
	dockerProtocol              string = "docker://"
	ociProtocol                 string = "oci://"
	releaseImageDir             string = "release-images"
	operatorImageDir            string = "operator-images"
	releaseImageExtractDir      string = "hold-release"
	releaseManifests            string = "release-manifests"
	imageReferences             string = "image-references"
	releaseImageExtractFullPath string = releaseImageExtractDir + "/" + releaseManifests + "/" + imageReferences
	blobsDir                    string = "/blobs/sha256/"
	errMsg                      string = "[ReleaseImageCollector] %v "
	diskToMirror                string = "diskToMirror"
	mirrorToDisk                string = "mirrorToDisk"
	logFile                     string = "logs/release.log"
)

type CollectorInterface interface {
	ReleaseImageCollector(ctx context.Context) ([]string, error)
}

func New(log clog.PluggableLoggerInterface,
	config v1alpha2.ImageSetConfiguration,
	opts mirror.CopyOptions,
	mirror mirror.MirrorInterface,
	manifest manifest.ManifestInterface,
	cincinnati CincinnatiInterface,
) CollectorInterface {
	return &Collector{Log: log, Config: config, Opts: opts, Mirror: mirror, Manifest: manifest, Cincinnati: cincinnati}
}

type Collector struct {
	Log        clog.PluggableLoggerInterface
	Mirror     mirror.MirrorInterface
	Manifest   manifest.ManifestInterface
	Config     v1alpha2.ImageSetConfiguration
	Opts       mirror.CopyOptions
	Cincinnati CincinnatiInterface
}

// ReleaseImageCollector - this looks into the operator index image
// taking into account the mode we are in (mirrorToDisk, diskToMirror)
// the image is downloaded (oci format) and the index.json is inspected
// once unmarshalled, the links to manifests are inspected
func (o *Collector) ReleaseImageCollector(ctx context.Context) ([]string, error) {

	var allImages []string

	if o.Opts.Mode == mirrorToDisk {
		releases := o.Cincinnati.GetReleaseReferenceImages(ctx)
		f, err := os.Create(logFile)
		if err != nil {
			o.Log.Error("[ReleaseImageCollector] %v", err)
		}
		if !strings.Contains(o.Opts.Destination, ociProtocol) {
			return []string{}, fmt.Errorf(" [ReleaseImageCollector] destination must use oci: or docker:// prefix")
		}
		dir := strings.Split(o.Opts.Destination, ":")[1]

		writer := bufio.NewWriter(f)
		defer f.Close()
		// dev mode debugging
		if !o.Opts.Dev {
			for key := range releases {
				o.Log.Info("copying image %s ", key)
				src := dockerProtocol + key
				dest := ociProtocol + workingDir + releaseImageDir
				err := o.Mirror.Run(ctx, src, dest, "copy", &o.Opts, *writer)
				if err != nil {
					return []string{}, fmt.Errorf(errMsg, err)
				}
				o.Log.Debug("copied release index image %s ", key)

				// TODO: create common function read the logs
				f, _ := os.ReadFile(logFile)
				lines := strings.Split(string(f), "\n")
				for _, s := range lines {
					if len(s) > 0 {
						o.Log.Debug(" %s ", strings.ToLower(s))
					}
				}
			}
		}

		oci, err := o.Manifest.GetImageIndex(workingDir + releaseImageDir)
		if err != nil {
			o.Log.Error("[ReleaseImageCollector] %v ", err)
			return []string{}, fmt.Errorf(errMsg, err)
		}

		//read the link to the manifest
		if len(oci.Manifests) == 0 {
			return []string{}, fmt.Errorf(errMsg, "image index not found ")
		}
		manifest := strings.Split(oci.Manifests[0].Digest, ":")[1]
		o.Log.Debug("image index %v", manifest)

		oci, err = o.Manifest.GetImageManifest(workingDir + releaseImageDir + blobsDir + manifest)
		if err != nil {
			return []string{}, fmt.Errorf(errMsg, err)
		}
		o.Log.Debug("manifest %v ", oci.Config.Digest)

		err = o.Manifest.ExtractLayersOCI(workingDir+releaseImageDir+blobsDir, workingDir+releaseImageExtractDir, releaseManifests, oci)
		if err != nil {
			return []string{}, fmt.Errorf(errMsg, err)
		}
		o.Log.Debug("extracted oci layer %s ", workingDir+releaseImageExtractDir)

		allRelatedImages, err := o.Manifest.GetReleaseSchema(workingDir + releaseImageExtractFullPath)
		if err != nil {
			return []string{}, fmt.Errorf(errMsg, err)
		}
		allImages, err = batcWorkerConverter(o.Log, dir, allRelatedImages)
		if err != nil {
			return []string{}, fmt.Errorf(errMsg, err)
		}
	}
	if o.Opts.Mode == diskToMirror {
		if len(o.Opts.Global.From) == 0 {
			return []string{}, fmt.Errorf(errMsg, "in diskToMirror mode please use the --from flag")
		}
		// check the directory to copy
		regex, e := regexp.Compile(indexJson)
		if e != nil {
			o.Log.Error(errMsg, e)
		}
		e = filepath.Walk(o.Opts.Global.From+"/"+releaseImageDir, func(path string, info os.FileInfo, err error) error {
			if err == nil && regex.MatchString(info.Name()) {
				ns := strings.Split(filepath.Dir(path), releaseImageDir)
				if len(ns) == 0 {
					return fmt.Errorf(errMsg, "no directory found for operator-images - please verify")
				} else {
					name := strings.Split(ns[1], "/")
					if len(name) != 2 {
						return fmt.Errorf(errMsg+" %s ", "operator name and related compents are incorrect", name)
					}
					src := ociProtocol + ns[0] + releaseImageDir + "/" + name[1]
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

// batchWorkerConverter convert RelatedImages to strings for batch worker
func batcWorkerConverter(log clog.PluggableLoggerInterface, dir string, images []v1alpha3.RelatedImage) ([]string, error) {
	var result []string
	for _, img := range images {
		src := dockerProtocol + img.Image
		dest := ociProtocol + workingDir + dir + "/" + releaseImageDir + "/" + img.Name
		err := os.MkdirAll(workingDir+dir+"/"+releaseImageDir+"/"+img.Name, 0750)
		if err != nil {
			log.Error("[batchWorkerConverter] %v", err)
			return []string{}, err
		}
		log.Debug("source %s ", src)
		log.Debug("destination %s ", dest)
		result = append(result, src+"*"+dest)
	}
	return result, nil
}
