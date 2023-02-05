package services

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/lmzuccarelli/golang-oci-mirror/pkg/api/v1alpha2"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/api/v1alpha3"
	"github.com/microlib/simple"
)

func ExecuteOperators(logger *simple.Logger, cfg v1alpha2.ImageSetConfiguration, opts CopyOptions) error {

	writer := bufio.NewWriter(os.Stdout)

	var images int
	var allRelatedImages []v1alpha3.RelatedImage
	var olm []v1alpha3.DeclarativeConfig

	compare := make(map[string]v1alpha3.ISCPackage)
	relatedImages := make(map[string][]v1alpha3.RelatedImage)
	bundles := make(map[string]bool)

	// compile a map to compare channels,min & max versions
	for _, ops := range cfg.Mirror.Operators {
		logger.Info(fmt.Sprintf("isc operators: %s\n", ops.Catalog))
		for _, pkg := range ops.Packages {
			logger.Info(fmt.Sprintf("catalog packages: %s \n", pkg.Name))
			for _, channel := range pkg.Channels {
				compare[pkg.Name] = v1alpha3.ISCPackage{Channel: channel.Name, MinVersion: channel.MinVersion, MaxVersion: channel.MaxVersion}
				logger.Info(fmt.Sprintf("channels: %v \n", compare))
			}
		}
	}

	for _, op := range cfg.Mirror.Operators {
		args := []string{dockerProtocol + op.Catalog, ociProtocol + workingDir + operatorImageDir, "true"}
		logger.Info(fmt.Sprintf("copying operator image %v", op.Catalog))
		err := Run(args, &opts, writer)
		if err != nil {
			logger.Error(fmt.Sprintf(" %v ", err))
		}

		oci, err := GetImageIndex(workingDir + operatorImageDir)
		if err != nil {
			logger.Error(fmt.Sprintf(" %v ", err))
			return err
		}

		//read the link to the manifest
		manifest := strings.Split(oci.Manifests[0].Digest, ":")[1]
		logger.Info(fmt.Sprintf("manifest %v", manifest))

		oci, err = GetImageManifest(workingDir + operatorImageDir + blobsDir + manifest)
		if err != nil {
			logger.Error(fmt.Sprintf(" %v ", err))
			return err
		}

		// read the config digest to get the detailed manifest
		ocs, err := GetOperatorConfig(workingDir + operatorImageDir + blobsDir + strings.Split(oci.Config.Digest, ":")[1])
		if err != nil {
			logger.Error(fmt.Sprintf(" %v ", err))
			return err
		}

		label := ocs.Config.Labels.OperatorsOperatorframeworkIoIndexConfigsV1

		logger.Info(fmt.Sprintf("label %s", label))

		for _, blob := range oci.Layers {
			f, err := os.Open(workingDir + operatorImageDir + blobsDir + strings.Split(blob.Digest, ":")[1])
			err = UntarLayers(f, operatorImageExtractDir, label)
			if err != nil {
				logger.Error(fmt.Sprintf(" %v ", err))
			}
		}

		// iterate through each package
		for _, pkg := range op.Packages {
			data, err := os.ReadFile(operatorImageExtractDir + "/" + label + "/" + pkg.Name + "/catalog.json")
			if err != nil {
				return err
			}

			// the catalog.json - does not really conform to json standards
			// this needs some thorough testing
			tmp := strings.NewReplacer(" ", "").Replace(string(data))
			updatedJson := "[" + strings.ReplaceAll(tmp, "}\n{", "},{") + "]"
			err = json.Unmarshal([]byte(updatedJson), &olm)
			if err != nil {
				return err
			}

			// iterate through the catalog object
			for i, obj := range olm {
				switch {
				case obj.Schema == "olm.channel":
					res := compare[obj.Package]
					if (v1alpha3.ISCPackage{}) != res {
						if res.Channel == obj.Name {
							logger.Info(fmt.Sprintf("found channel : %v", obj))
							logger.Info(fmt.Sprintf("bundle image to use : %v", obj.Entries[0].Name))
							bundles[obj.Entries[0].Name] = true
						}
					}
				case obj.Schema == "olm.bundle":
					if bundles[obj.Name] {
						logger.Trace(fmt.Sprintf("config bundle: %d %v", i, obj.Name))
						logger.Trace(fmt.Sprintf("config relatedImages: %d %v", i, obj.RelatedImages))
						relatedImages[obj.Name] = obj.RelatedImages
					}
				case obj.Schema == "olm.package":
					logger.Info(fmt.Sprintf("Config package: %v", obj.Name))
				}
			}
			logger.Trace(fmt.Sprintf("related images %v", relatedImages))
		}
	}

	logger.Info("images to copy")
	for _, v := range relatedImages {
		images = images + len(v)
		for _, x := range v {
			logger.Info(fmt.Sprintf("  %s ", x.Image))
			allRelatedImages = append(allRelatedImages, x)
		}
	}

	type BatchSchema struct {
		items     int
		count     int
		batchSize int
		remainder int
	}

	var batch *BatchSchema

	if images < BATCH_SIZE {
		batch = &BatchSchema{items: images, count: 0, batchSize: 1, remainder: images}
	} else {
		batch = &BatchSchema{items: images, count: (images / BATCH_SIZE), batchSize: BATCH_SIZE, remainder: (images % BATCH_SIZE)}
	}

	logger.Info(fmt.Sprintf("images to mirror %d", batch.items))
	logger.Info(fmt.Sprintf("batch size %d", batch.batchSize))
	logger.Info(fmt.Sprintf("total batches %d", batch.count))
	logger.Info(fmt.Sprintf("remainder size %d", batch.remainder))
	fmt.Println(" ")

	// main loop
	var errArray []error
	var runArgs []string

	var wg sync.WaitGroup
	wg.Add(batch.batchSize)
	for i := 0; i < batch.count; i++ {
		logger.Info(fmt.Sprintf("starting batch %d ", i))
		for x := 0; x < batch.batchSize; x++ {
			index := (i * batch.batchSize) + x
			irs, err := customImageParser(allRelatedImages[index].Image)
			if err != nil {
				logger.Error(fmt.Sprintf("%v", err))
				continue
			}
			// ignore the failure as it will be picked up in the Run
			if strings.Contains(opts.Global.Destination, ociProtocol) {
				os.MkdirAll(strings.Split(opts.Global.Destination, ":")[1]+"/"+irs.Namespace, 0750)
				runArgs = []string{dockerProtocol + allRelatedImages[index].Image, opts.Global.Destination + "/" + irs.Namespace + "/" + irs.Component, "true"}
			} else if strings.Contains(opts.Global.Destination, dockerProtocol) {
				runArgs = []string{ociProtocol + allRelatedImages[index].Image, opts.Global.Destination, "true"}
			}
			logger.Debug(fmt.Sprintf("mirroring image %s -> %s", allRelatedImages[index].Image, opts.Global.Destination+"/"+irs.Namespace+"/"+irs.Component))
			go func() {
				defer wg.Done()
				err := Run(runArgs, &opts, writer)
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

	// complete remainder
	var wgRemainder sync.WaitGroup
	wgRemainder.Add(batch.remainder)
	logger.Info(fmt.Sprintf("starting batch %d  (remainder)", batch.batchSize))
	for x := 0; x < batch.remainder; x++ {
		index := (batch.count * BATCH_SIZE) + x
		irs, err := customImageParser(allRelatedImages[index].Image)
		if err != nil {
			logger.Error(fmt.Sprintf("%v", err))
			continue
		}
		// ignore the failure as it will be picked up in the Run
		if strings.Contains(opts.Global.Destination, ociProtocol) {
			os.MkdirAll(strings.Split(opts.Global.Destination, ":")[1]+"/"+irs.Namespace, 0750)
			runArgs = []string{dockerProtocol + allRelatedImages[index].Image, opts.Global.Destination + "/" + irs.Namespace + "/" + irs.Component, "true"}
			logger.Debug(fmt.Sprintf("mirroring image %v", runArgs))
		} else if strings.Contains(opts.Global.Destination, dockerProtocol) {
			runArgs = []string{ociProtocol + allRelatedImages[index].Image, opts.Global.Destination, "true"}
		}
		go func() {
			defer wgRemainder.Done()
			err := Run(runArgs, &opts, writer)
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
	logger.Info(fmt.Sprintf("completed batch %d (remainder)", batch.batchSize))
	return nil
}
