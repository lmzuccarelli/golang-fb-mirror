package services

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/lmzuccarelli/golang-oci-mirror/pkg/api/v1alpha2"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/api/v1alpha3"
	"github.com/microlib/simple"
)

func ExecuteRelease(logger *simple.Logger, cfg v1alpha2.ImageSetConfiguration, opts CopyOptions) error {

	mo := MirrorOptions{}
	writer := bufio.NewWriter(os.Stdout)
	ctx := context.Background()

	releaseOpts := NewReleaseOptions(&mo)
	releases := releaseOpts.Run(ctx, &cfg)

	for key := range releases {
		args := []string{dockerProtocol + key, ociProtocol + workingDir + releaseImageDir, "true"}
		logger.Info(fmt.Sprintf("copying image %s ", key))
		err := Run(args, &opts, writer)
		if err != nil {
			logger.Error(fmt.Sprintf(" %v ", err))
		}
	}

	oci, err := GetImageIndex(workingDir + releaseImageDir)
	if err != nil {
		logger.Error(fmt.Sprintf(" %v ", err))
		return err
	}

	//read the link to the manifest
	manifest := strings.Split(oci.Manifests[0].Digest, ":")[1]
	logger.Info(fmt.Sprintf("manifest %v", manifest))

	oci, err = GetImageManifest(workingDir + releaseImageDir + blobsDir + manifest)
	if err != nil {
		logger.Error(fmt.Sprintf(" %v ", err))
		return err
	}

	for _, blob := range oci.Layers {
		f, err := os.Open(workingDir + releaseImageDir + blobsDir + strings.Split(blob.Digest, ":")[1])
		err = UntarLayers(f, releaseImageExtractDir, releaseManifests)
		if err != nil {
			logger.Error(fmt.Sprintf(" %v ", err))
		}
	}

	var release = v1alpha3.ReleaseSchema{}

	file, _ := os.ReadFile(workingDir + releaseImageExtractFullPath)
	err = json.Unmarshal([]byte(file), &release)
	if err != nil {
		logger.Error(fmt.Sprintf("Unmarshaling to struct %v", err))
	}

	loop := len(release.Spec.Tags) / BATCH_SIZE
	logger.Info(fmt.Sprintf("Images to mirror %d", len(release.Spec.Tags)))
	logger.Info(fmt.Sprintf("Batch size %d", BATCH_SIZE))
	logger.Info(fmt.Sprintf("Total batches %d", loop))

	for _, item := range release.Spec.Tags {
		logger.Info(fmt.Sprintf("  %s ", item.Name))
	}

	fmt.Println(" ")

	// main loop
	fmt.Println(" ")
	var errArray []error
	var wg sync.WaitGroup
	wg.Add(BATCH_SIZE)
	for i := 0; i < loop; i++ {
		logger.Info(fmt.Sprintf("starting batch %d ", i))
		for batch := 0; batch < BATCH_SIZE; batch++ {
			index := (i * BATCH_SIZE) + batch
			args := []string{dockerProtocol + release.Spec.Tags[index].From.Name, ociProtocol + workingDir + "test-lmz/" + release.Spec.Tags[index].Name, "true"}
			logger.Debug(fmt.Sprintf("mirroring image %s -> %s", release.Spec.Tags[index].Name, strings.Split(release.Spec.Tags[index].From.Name, ":")[1]))
			go func() {
				defer wg.Done()
				err := Run(args, &opts, writer)
				if err != nil {
					errArray = append(errArray, err)
					logger.Error(fmt.Sprintf("%v", err))
				}
				writer.Flush()
			}()
		}
		wg.Wait()
		if len(errArray) > 0 {
			for _, err := range errArray {
				logger.Error(fmt.Sprintf("%v", err))
			}
		}
		logger.Info(fmt.Sprintf("completed batch %d", i))
		wg.Add(BATCH_SIZE)
		fmt.Println(" ")
	}

	remainder := len(release.Spec.Tags) % BATCH_SIZE

	// complete remainder
	var wgRemainder sync.WaitGroup
	wgRemainder.Add(remainder)
	logger.Info(fmt.Sprintf("starting batch %d  (remainder)", loop))
	for batch := 0; batch < remainder; batch++ {
		index := (loop * BATCH_SIZE) + batch
		args := []string{dockerProtocol + release.Spec.Tags[index].From.Name, ociProtocol + workingDir + "test-lmz/" + release.Spec.Tags[index].Name, "true"}
		logger.Debug(fmt.Sprintf("mirroring image %s -> %s", release.Spec.Tags[index].Name, strings.Split(release.Spec.Tags[index].From.Name, ":")[1]))
		go func() {
			defer wgRemainder.Done()
			err := Run(args, &opts, writer)
			if err != nil {
				errArray = append(errArray, err)
				logger.Error(fmt.Sprintf("%v", err))
			}
			writer.Flush()
		}()
	}
	wgRemainder.Wait()
	if len(errArray) > 0 {
		for _, err := range errArray {
			logger.Error(fmt.Sprintf("%v", err))
		}
	}
	logger.Info(fmt.Sprintf("completed batch %d (remainder)", loop))
	return nil
}
