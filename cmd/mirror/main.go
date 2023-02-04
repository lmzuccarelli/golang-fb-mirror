package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/lmzuccarelli/golang-oci-mirror/pkg/config"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/schema"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/services"
	"github.com/microlib/simple"
)

const (
	dockerProtocol              string = "docker://"
	ociProtocol                 string = "oci:"
	releaseImageDir             string = "release-images"
	releaseImageExtractDir      string = "hold-release"
	releaseManifests            string = "release-manifests"
	imageReferences             string = "image-references"
	releaseImageExtractFullPath string = releaseImageExtractDir + "/" + releaseManifests + "/" + imageReferences
	blobsDir                    string = "/blobs/sha256/"
)

func main() {

	const (
		BATCH_SIZE int = 8
	)

	//var release schema.ReleaseSchema
	logger := &simple.Logger{Level: "debug"}

	cfg, err := config.ReadConfig("isc.yaml")
	if err != nil {
		logger.Error(fmt.Sprintf("ISC %v ", err))
	}
	logger.Debug(fmt.Sprintf("ISC : %v", cfg))

	mo := services.MirrorOptions{}
	//mo.init(cfg)

	global := &services.GlobalOptions{Debug: true, TlsVerify: false, InsecurePolicy: true}
	_, sharedOpts := services.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := services.DeprecatedTLSVerifyFlags()
	_, srcOpts := services.ImageFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	_, destOpts := services.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	_, retryOpts := services.RetryFlags()
	opts := services.CopyOptions{
		Global:              global,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOpts,
		DestImage:           destOpts,
		RetryOpts:           retryOpts,
	}

	writer := bufio.NewWriter(os.Stdout)
	ctx := context.Background()

	releaseOpts := services.NewReleaseOptions(&mo)
	releases := releaseOpts.Run(ctx, &cfg)

	for key := range releases {
		args := []string{dockerProtocol + key, ociProtocol + releaseImageDir, "true"}
		logger.Info(fmt.Sprintf("copying image %s ", key))
		err = services.Run(args, &opts, writer)
		if err != nil {
			logger.Error(fmt.Sprintf(" %v ", err))
		}
	}

	oci, err := services.GetImageIndex(releaseImageDir)
	if err != nil {
		logger.Error(fmt.Sprintf(" %v ", err))
		os.Exit(1)
	}

	//read the link to the manifest
	manifest := strings.Split(oci.Manifests[0].Digest, ":")[1]
	logger.Info(fmt.Sprintf("manifest %v", manifest))

	oci, err = services.GetImageManifest(releaseImageDir + blobsDir + manifest)
	if err != nil {
		logger.Error(fmt.Sprintf(" %v ", err))
		os.Exit(1)
	}

	for _, blob := range oci.Layers {
		f, err := os.Open(releaseImageDir + blobsDir + strings.Split(blob.Digest, ":")[1])
		err = services.UntarLayers(f, releaseImageExtractDir, releaseManifests)
		if err != nil {
			logger.Error(fmt.Sprintf(" %v ", err))
		}
	}

	var release = schema.ReleaseSchema{}

	file, _ := os.ReadFile(releaseImageExtractFullPath)
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
			args := []string{dockerProtocol + release.Spec.Tags[index].From.Name, ociProtocol + "test-lmz/" + release.Spec.Tags[index].Name, "true"}
			logger.Debug(fmt.Sprintf("mirroring image %s -> %s", release.Spec.Tags[index].Name, strings.Split(release.Spec.Tags[index].From.Name, ":")[1]))
			go func() {
				defer wg.Done()
				err := services.Run(args, &opts, writer)
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
		args := []string{dockerProtocol + release.Spec.Tags[index].From.Name, ociProtocol + "test-lmz/" + release.Spec.Tags[index].Name, "true"}
		logger.Debug(fmt.Sprintf("mirroring image %s -> %s", release.Spec.Tags[index].Name, strings.Split(release.Spec.Tags[index].From.Name, ":")[1]))
		go func() {
			defer wgRemainder.Done()
			err := services.Run(args, &opts, writer)
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

}
