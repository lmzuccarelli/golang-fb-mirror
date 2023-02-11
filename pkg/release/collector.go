package release

import (
	"bufio"
	"context"
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
	ReleaseImageCollector(ctx context.Context) ([]v1alpha3.RelatedImage, error)
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
func (o *Collector) ReleaseImageCollector(ctx context.Context) ([]v1alpha3.RelatedImage, error) {
	writer := bufio.NewWriter(os.Stdout)
	releases := o.Cincinnati.GetReleaseReferenceImages(ctx)

	for key := range releases {
		//args := []string{dockerProtocol + key, ociProtocol + workingDir + releaseImageDir, "true"}
		o.Log.Info("copying image %s ", key)
		src := dockerProtocol + key
		dest := ociProtocol + workingDir + releaseImageDir
		err := o.Mirror.Run(ctx, src, dest, &o.Opts, writer)
		if err != nil {
			return []v1alpha3.RelatedImage{}, err
		}
	}

	oci, err := o.Manifest.GetImageIndex(workingDir + releaseImageDir)
	if err != nil {
		o.Log.Error(" %v ", err)
		return []v1alpha3.RelatedImage{}, err
	}

	//read the link to the manifest
	manifest := strings.Split(oci.Manifests[0].Digest, ":")[1]
	o.Log.Info("manifest %v", manifest)

	oci, err = o.Manifest.GetImageManifest(workingDir + releaseImageDir + blobsDir + manifest)
	if err != nil {
		return []v1alpha3.RelatedImage{}, err
	}

	err = o.Manifest.ExtractLayersOCI(releaseImageExtractDir, releaseManifests, oci)
	if err != nil {
		return []v1alpha3.RelatedImage{}, err
	}

	return o.Manifest.GetReleaseSchema(workingDir + releaseImageExtractFullPath)

}
