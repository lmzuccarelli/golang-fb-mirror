package operator

import (
	"bufio"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

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
	diskToMirror                string = "diskToMirror"
	mirrorToDisk                string = "mirrorToDisk"
	errMsg                      string = "[OperatorImageCollector] %v "
	logsFile                    string = "logs/operator.log"
)

type CollectorInterface interface {
	OperatorImageCollector(ctx context.Context) ([]string, error)
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

// OperatorImageCollector - this looks into the operator index image
// taking into account the mode we are in (mirrorToDisk, diskToMirror)
// the image is downloaded (oci format) and the index.json is inspected
// once unmarshalled, the links to manifests are inspected
func (o *Collector) OperatorImageCollector(ctx context.Context) ([]string, error) {

	var allImages []string
	compare := make(map[string]v1alpha3.ISCPackage)
	relatedImages := make(map[string][]v1alpha3.RelatedImage)
	label := "configs"
	if !strings.Contains(o.Opts.Destination, ociProtocol) && !strings.Contains(o.Opts.Destination, dockerProtocol) {
		return []string{}, fmt.Errorf(errMsg, "destination must use oci:// or docker:// prefix")
	}
	dir := strings.Split(o.Opts.Destination, ":")[1]

	// compile a map to compare channels,min & max versions
	for _, ops := range o.Config.Mirror.Operators {
		o.Log.Info("isc operators: %s\n", ops.Catalog)
		for _, pkg := range ops.Packages {
			o.Log.Info("catalog packages: %s \n", pkg.Name)
			for _, channel := range pkg.Channels {
				compare[pkg.Name] = v1alpha3.ISCPackage{Channel: channel.Name, MinVersion: channel.MinVersion, MaxVersion: channel.MaxVersion}
				o.Log.Info("channels: %v \n", compare)
			}
		}
	}

	// check the mode
	if o.Opts.Mode == mirrorToDisk {
		f, err := os.Create(logsFile)
		if err != nil {
			o.Log.Error(errMsg, err)
		}
		writer := bufio.NewWriter(f)
		defer f.Close()
		for _, op := range o.Config.Mirror.Operators {

			if !o.Opts.Dev {
				// download the operator index image
				o.Log.Info("copying operator image %v", op.Catalog)
				src := dockerProtocol + op.Catalog
				dest := ociProtocol + workingDir + operatorImageDir
				err := o.Mirror.Run(ctx, src, dest, "copy", &o.Opts, *writer)
				writer.Flush()
				if err != nil {
					o.Log.Error(errMsg, err)
				}
				// read the logs
				f, _ := os.ReadFile(logsFile)
				lines := strings.Split(string(f), "\n")
				for _, s := range lines {
					if len(s) > 0 {
						o.Log.Debug("%s ", strings.ToLower(s))
					}
				}

				// it's in oci format so we can go directly to the index.json file
				oci, err := o.Manifest.GetImageIndex(workingDir + operatorImageDir)
				if err != nil {
					return []string{}, err
				}

				//read the link to the manifest
				if len(oci.Manifests) == 0 {
					return []string{}, fmt.Errorf("[OperatorImageCollector] no manifests found for %s ", op.Catalog)
				} else {
					if !strings.Contains(oci.Manifests[0].Digest, "sha256") {
						return []string{}, fmt.Errorf("[OperatorImageCollector] the disgets seems to incorrect for %s ", op.Catalog)
					}
				}
				manifest := strings.Split(oci.Manifests[0].Digest, ":")[1]
				o.Log.Info("manifest %v", manifest)

				// read the operator image manifest
				oci, err = o.Manifest.GetImageManifest(workingDir + operatorImageDir + blobsDir + manifest)
				if err != nil {
					return []string{}, err
				}

				// read the config digest to get the detailed manifest
				// looking for the lable to search for a specific folder
				ocs, err := o.Manifest.GetOperatorConfig(workingDir + operatorImageDir + blobsDir + strings.Split(oci.Config.Digest, ":")[1])
				if err != nil {
					return []string{}, err
				}

				label = ocs.Config.Labels.OperatorsOperatorframeworkIoIndexConfigsV1
				o.Log.Info("label %s", label)

				// untar all the blobs for the operator
				// if the layer with "label (from previous step) is found to a specific folder"
				err = o.Manifest.ExtractLayersOCI(workingDir+operatorImageDir+blobsDir, workingDir+operatorImageExtractDir, label, oci)
				if err != nil {
					return []string{}, err
				}
			}

			// select all packages
			// this is the equivalent of the headOnly mode
			// only the latest version of each operator will be selected
			if len(op.Packages) == 0 {
				relatedImages, err = o.Manifest.GetRelatedImagesFromCatalog(operatorImageExtractDir, label)
				if err != nil {
					return []string{}, err
				}
			} else {
				// iterate through each package
				relatedImages, err = o.Manifest.GetRelatedImagesFromCatalogByFilter(operatorImageExtractDir, label, op, compare)
				if err != nil {
					return []string{}, err
				}
			}
		}
		o.Log.Info("related images length %d ", len(relatedImages))
		var count = 0
		for _, v := range relatedImages {
			count = count + len(v)
		}
		o.Log.Info("images to copy (before duplicates) %d ", count)

		// remove all duplicates
		var cleanedImages []v1alpha3.RelatedImage
		imgs := cleanDuplicates(relatedImages)
		o.Log.Trace("flatenned %v ", imgs)
		o.Log.Debug("images to copy")
		for k, v := range imgs {
			o.Log.Debug("  name %s", v)
			o.Log.Debug("  image %s", k)
			cleanedImages = append(cleanedImages, v1alpha3.RelatedImage{Name: v, Image: k})
		}
		allImages, err = batchWorkerConverter(o.Log, dir, cleanedImages)
		if err != nil {
			return []string{}, err
		}
	}

	if o.Opts.Mode == diskToMirror {
		if len(o.Opts.Global.From) == 0 {
			return []string{}, fmt.Errorf(errMsg, "in diskToMirror mode please use the --from flag")
		}
		// check the directory to copy
		regex, e := regexp.Compile(indexJson)
		if e != nil {
			o.Log.Error("%v", e)
		}
		e = filepath.Walk(workingDir+"/"+o.Opts.Global.From+"/"+operatorImageDir, func(path string, info os.FileInfo, err error) error {
			if err == nil && regex.MatchString(info.Name()) {
				ns := strings.Split(filepath.Dir(path), operatorImageDir)
				if len(ns) == 0 {
					return fmt.Errorf(errMsg+"%s", "no directory found for operator-images ", path)
				} else {
					name := strings.Split(ns[1], "/")
					if len(name) != 3 {
						return fmt.Errorf(errMsg+"%s", "operator name and related compents are incorrect", name)
					}
					src := strings.Trim(ociProtocol, "/") + ns[0] + operatorImageDir + "/" + name[1] + "/" + name[2]
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

// cleanDuplicates - simple utility to remove duplicates
func cleanDuplicates(m map[string][]v1alpha3.RelatedImage) map[string]string {
	x := make(map[string]string)
	for _, v := range m {
		for _, ri := range v {
			x[ri.Image] = ri.Name
		}
	}
	return x
}

// customImageParser - simple image string parser
func customImageParser(image string) (*v1alpha3.ImageRefSchema, error) {
	var irs *v1alpha3.ImageRefSchema
	var component string
	parts := strings.Split(image, "/")
	if len(parts) < 3 {
		return irs, fmt.Errorf("[customImageParser] image url seems to be wrong %s ", image)
	}
	if strings.Contains(parts[2], "@") {
		component = strings.Split(parts[2], "@")[0]
	} else {
		component = parts[2]
	}
	irs = &v1alpha3.ImageRefSchema{Repository: parts[0], Namespace: parts[1], Component: component}
	return irs, nil
}

// batchWorkerConverter convert RelatedImages to strings for batch worker
func batchWorkerConverter(log clog.PluggableLoggerInterface, dir string, images []v1alpha3.RelatedImage) ([]string, error) {
	var result []string
	for _, img := range images {
		irs, err := customImageParser(img.Image)
		if err != nil {
			log.Error("[batchWorkerConverter] %v", err)
			return result, err
		}
		err = os.MkdirAll(workingDir+dir+"/"+operatorImageDir+"/"+irs.Namespace, 0750)
		if err != nil {
			log.Error("[batchWorkerConverter] %v", err)
			return result, err
		}
		src := dockerProtocol + img.Image
		if len(img.Name) == 0 {
			timestamp := time.Now().Unix()
			s := fmt.Sprintf("%d", timestamp)
			img.Name = fmt.Sprintf("%x", sha256.Sum256([]byte(s)))[:6]
		}
		dest := ociProtocol + workingDir + dir + "/" + operatorImageDir + "/" + irs.Namespace + "/" + img.Name
		log.Debug("source %s ", img.Image)
		log.Debug("destination %s ", workingDir+dir+"/"+operatorImageDir+"/"+irs.Namespace+"/"+img.Name)
		result = append(result, src+"*"+dest)
	}
	return result, nil
}
