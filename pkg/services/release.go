package services

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"strings"

	"github.com/lmzuccarelli/golang-oci-mirror/pkg/api/v1alpha3"
)

func (o *Executor) ExecuteRelease(ctx context.Context) error {

	mo := MirrorOptions{}
	writer := bufio.NewWriter(os.Stdout)

	releaseOpts := NewReleaseOptions(&mo)
	releases := releaseOpts.Run(ctx, &o.Config)

	for key := range releases {
		//args := []string{dockerProtocol + key, ociProtocol + workingDir + releaseImageDir, "true"}
		o.Log.Info("copying image %s ", key)
		src := dockerProtocol + key
		dest := ociProtocol + workingDir + releaseImageDir
		err := o.Mirror.Run(ctx, src, dest, &o.Opts, writer)
		if err != nil {
			o.Log.Error(" %v ", err)
		}
	}

	oci, err := o.Manifest.GetImageIndex(workingDir + releaseImageDir)
	if err != nil {
		o.Log.Error(" %v ", err)
		return err
	}

	//read the link to the manifest
	manifest := strings.Split(oci.Manifests[0].Digest, ":")[1]
	o.Log.Info("manifest %v", manifest)

	oci, err = o.Manifest.GetImageManifest(workingDir + releaseImageDir + blobsDir + manifest)
	if err != nil {
		o.Log.Error(" %v ", err)
		return err
	}

	err = o.Manifest.ExtractLayersOCI(releaseImageExtractDir, releaseManifests, oci)
	if err != nil {
		o.Log.Error(" %v ", err)
	}

	var release = v1alpha3.ReleaseSchema{}

	file, _ := os.ReadFile(workingDir + releaseImageExtractFullPath)
	err = json.Unmarshal([]byte(file), &release)
	if err != nil {
		o.Log.Error("Unmarshaling to struct %v", err)
	}

	var allImages []v1alpha3.RelatedImage
	for _, item := range release.Spec.Tags {
		o.Log.Info("  %s ", item.Name)
		allImages = append(allImages, v1alpha3.RelatedImage{Image: item.Name})
	}

	var batch *BatchSchema
	images := len(release.Spec.Tags)
	if images < BATCH_SIZE {
		batch = &BatchSchema{items: images, count: 1, batchSize: images, batchIndex: 0, remainder: 0}
	} else {
		batch = &BatchSchema{items: images, count: (images / BATCH_SIZE), batchSize: BATCH_SIZE, remainder: (images % BATCH_SIZE)}
	}
	batch.copyImages = allImages
	batch.Writer = writer

	//TODO
	// add these in the BatchWorker
	// src := dockerProtocol + release.Spec.Tags[index].From.Name
	//	dest := strings.Split(release.Spec.Tags[index].From.Name, ":")[1]

	//call the batch executioner
	err = o.BatchWorker(ctx, batch, o.Opts)
	if err != nil {
		return err
	}
	if batch.remainder > 0 {
		batch.batchIndex = batch.count
		batch.count = batch.remainder
		batch.batchSize = 1
		err := o.BatchWorker(ctx, batch, o.Opts)
		if err != nil {
			return err
		}
	}
	writer.Flush()
	return nil
}
