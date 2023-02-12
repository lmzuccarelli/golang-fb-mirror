package batch

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/lmzuccarelli/golang-oci-mirror/pkg/api/v1alpha2"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/api/v1alpha3"
	clog "github.com/lmzuccarelli/golang-oci-mirror/pkg/log"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/manifest"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/mirror"
)

const (
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

type BatchInterface interface {
	Worker(ctx context.Context, images []v1alpha3.RelatedImage, opts mirror.CopyOptions) error
}

func New(log clog.PluggableLoggerInterface,
	config v1alpha2.ImageSetConfiguration,
	opts mirror.CopyOptions,
	mirror mirror.MirrorInterface,
	manifest manifest.ManifestInterface,
) BatchInterface {
	return &Batch{Log: log, Config: config, Opts: opts, Mirror: mirror, Manifest: manifest}
}

type Batch struct {
	Log      clog.PluggableLoggerInterface
	Mirror   mirror.MirrorInterface
	Manifest manifest.ManifestInterface
	Config   v1alpha2.ImageSetConfiguration
	Opts     mirror.CopyOptions
}

type BatchSchema struct {
	Writer     io.Writer
	CopyImages []v1alpha3.RelatedImage
	Items      int
	Count      int
	BatchSize  int
	BatchIndex int
	Remainder  int
}

func (o *Batch) Worker(ctx context.Context, images []v1alpha3.RelatedImage, opts mirror.CopyOptions) error {

	var errArray []error
	var wg sync.WaitGroup

	var b *BatchSchema
	imgs := len(images)
	if imgs < BATCH_SIZE {
		b = &BatchSchema{Items: imgs, Count: 1, BatchSize: imgs, BatchIndex: 0, Remainder: 0}
	} else {
		b = &BatchSchema{Items: imgs, Count: (imgs / BATCH_SIZE), BatchSize: BATCH_SIZE, Remainder: (imgs % BATCH_SIZE)}
	}

	writer := bufio.NewWriter(os.Stdout)
	o.Log.Info("images to mirror %d", b.Items)
	o.Log.Info("batch count %d", b.Count)
	o.Log.Info("batch index %d", b.BatchIndex)
	o.Log.Info("batch size %d", b.BatchSize)
	o.Log.Info("remainder size %d", b.Remainder)

	b.CopyImages = images
	// prepare batching

	wg.Add(b.BatchSize)
	for i := 0; i < b.Count; i++ {
		o.Log.Info(fmt.Sprintf("starting batch %d ", i))
		for x := 0; x < b.BatchSize; x++ {
			index := (i * b.BatchSize) + x
			irs, err := customImageParser(b.CopyImages[index].Image)
			if err != nil {
				o.Log.Error("%v", err)
				continue
			}
			// ignore the failure as it will be picked up in the Run
			err = os.MkdirAll(strings.Split(opts.Destination, ":")[1]+"/"+irs.Namespace, 0750)
			if err != nil {
				return err
			}
			src := dockerProtocol + b.CopyImages[index].Image
			dest := opts.Destination + "/" + irs.Namespace + "/" + irs.Component

			o.Log.Debug("source %s ", b.CopyImages[index].Image)
			o.Log.Debug("destination %s ", opts.Destination+"/"+irs.Namespace+"/"+irs.Component)

			go func(ctx context.Context, src, dest string, opts *mirror.CopyOptions, writer io.Writer) {
				defer wg.Done()
				err := o.Mirror.Run(ctx, src, dest, opts, writer)
				if err != nil {
					errArray = append(errArray, err)
				}
			}(ctx, src, dest, &opts, writer)
		}
		wg.Wait()
		writer.Flush()
		o.Log.Info("completed batch %d", i)
		if b.Count > 1 {
			wg.Add(BATCH_SIZE)
		}
		if len(errArray) > 0 {
			for _, err := range errArray {
				o.Log.Error(" errArray %v", err)
			}
			return fmt.Errorf("error in batch - refer to console logs")
		}
	}
	if b.Remainder > 0 {
		//b.BatchIndex = b.Count
		//b.Count = b.Remainder
		//b.BatchSize = 1
		// one level of simple recursion
		err := o.Worker(ctx, images[b.Count:], o.Opts)
		if err != nil {
			return err
		}
	}
	return nil
}

func customImageParser(image string) (*v1alpha3.ImageRefSchema, error) {
	var irs *v1alpha3.ImageRefSchema
	var component string
	parts := strings.Split(image, "/")
	if len(parts) < 3 {
		return irs, fmt.Errorf("image url seems to be wrong %s ", image)
	}
	if strings.Contains(parts[2], "@") {
		component = strings.Split(parts[2], "@")[0]
	} else {
		component = parts[2]
	}
	irs = &v1alpha3.ImageRefSchema{Repository: parts[0], Namespace: parts[1], Component: component}
	return irs, nil
}
