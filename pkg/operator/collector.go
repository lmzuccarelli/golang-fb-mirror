package operator

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/lmzuccarelli/golang-oci-mirror/pkg/api/v1alpha2"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/api/v1alpha3"
	clog "github.com/lmzuccarelli/golang-oci-mirror/pkg/log"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/manifest"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/mirror"
)

const (
	catalogJson string = "catalog.json"
	// TODO: make this global
	operatorImageExtractDir     string = "hold-operator"
	workingDir                  string = "working-dir/"
	dockerProtocol              string = "docker://"
	ociProtocol                 string = "oci:"
	releaseImageDir             string = "release-images"
	operatorImageDir            string = "operator-images"
	releaseImageExtractDir      string = "hold-release"
	releaseManifests            string = "release-manifests"
	imageReferences             string = "image-references"
	releaseImageExtractFullPath string = releaseImageExtractDir + "/" + releaseManifests + "/" + imageReferences
	blobsDir                    string = "/blobs/sha256/"
	BATCH_SIZE                  int    = 8
	diskToMirror                string = "diskToMirror"
	mirrorToDisk                string = "mirrorToDisk"
)

type CollectorInterface interface {
	OperatorImageCollector(ctx context.Context) ([]v1alpha3.RelatedImage, error)
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
func (o *Collector) OperatorImageCollector(ctx context.Context) ([]v1alpha3.RelatedImage, error) {

	var allRelatedImages []v1alpha3.RelatedImage
	compare := make(map[string]v1alpha3.ISCPackage)
	relatedImages := make(map[string][]v1alpha3.RelatedImage)
	var err error
	label := "configs"

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
		writer := bufio.NewWriter(os.Stdout)
		for _, op := range o.Config.Mirror.Operators {

			if !o.Opts.Dev {
				// download the operator index image
				o.Log.Info("copying operator image %v", op.Catalog)
				src := dockerProtocol + op.Catalog
				dest := ociProtocol + workingDir + operatorImageDir
				err := o.Mirror.Run(ctx, src, dest, &o.Opts, writer)
				writer.Flush()
				if err != nil {
					o.Log.Error(" %v ", err)
				}
				// it's in oci format so we can go directly to the index.json file
				oci, err := o.Manifest.GetImageIndex(workingDir + operatorImageDir)
				if err != nil {
					return []v1alpha3.RelatedImage{}, err
				}

				//read the link to the manifest
				if len(oci.Manifests) == 0 {
					return []v1alpha3.RelatedImage{}, fmt.Errorf("no manifests found for %s ", op.Catalog)
				} else {
					if !strings.Contains(oci.Manifests[0].Digest, "sha256") {
						return []v1alpha3.RelatedImage{}, fmt.Errorf("the disgets seems to incorrect for %s ", op.Catalog)
					}
				}
				manifest := strings.Split(oci.Manifests[0].Digest, ":")[1]
				o.Log.Info("manifest %v", manifest)

				// read the operator image manifest
				oci, err = o.Manifest.GetImageManifest(workingDir + operatorImageDir + blobsDir + manifest)
				if err != nil {
					return []v1alpha3.RelatedImage{}, err
				}

				// read the config digest to get the detailed manifest
				// looking for the lable to search for a specific folder
				ocs, err := o.Manifest.GetOperatorConfig(workingDir + operatorImageDir + blobsDir + strings.Split(oci.Config.Digest, ":")[1])
				if err != nil {
					return []v1alpha3.RelatedImage{}, err
				}

				label = ocs.Config.Labels.OperatorsOperatorframeworkIoIndexConfigsV1

				o.Log.Info("label %s", label)

				// untar all the blobs for the operator
				// if the layer with "label (from previous step) is found to a specific folder"
				err = o.Manifest.ExtractLayersOCI(workingDir+operatorImageDir+blobsDir, label, oci)
				if err != nil {
					return []v1alpha3.RelatedImage{}, err
				}
			}

			// select all packages
			// this is the equivalent of the headOnly mode
			// only the latest version of each operator will be selected
			if len(op.Packages) == 0 {
				relatedImages, err = o.Manifest.GetRelatedImagesFromCatalog(operatorImageExtractDir, label)
				if err != nil {
					return []v1alpha3.RelatedImage{}, err
				}
			} else {
				// iterate through each package
				relatedImages, err = o.Manifest.GetRelatedImagesFromCatalogByFilter(operatorImageExtractDir, label, op, compare)
				if err != nil {
					return []v1alpha3.RelatedImage{}, err
				}
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
	imgs := cleanDuplicates(relatedImages)
	o.Log.Trace("flatenned %v ", imgs)
	o.Log.Debug("images to copy")
	for k, v := range imgs {
		o.Log.Debug("  name %s", v)
		o.Log.Debug("  image %s", k)
		allRelatedImages = append(allRelatedImages, v1alpha3.RelatedImage{Name: v, Image: k})
	}
	return allRelatedImages, nil
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
