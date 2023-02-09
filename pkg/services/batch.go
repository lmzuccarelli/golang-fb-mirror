package services

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/lmzuccarelli/golang-oci-mirror/pkg/api/v1alpha3"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/mirror"
)

type BatchSchema struct {
	Writer     io.Writer
	copyImages []v1alpha3.RelatedImage
	items      int
	count      int
	batchSize  int
	batchIndex int
	remainder  int
}

func (o *Executor) BatchWorker(ctx context.Context, workload *BatchSchema, opts mirror.CopyOptions) error {

	var errArray []error
	var wg sync.WaitGroup

	writer := bufio.NewWriter(os.Stdout)
	o.Log.Info("images to mirror %d", workload.items)
	o.Log.Info("batch count %d", workload.count)
	o.Log.Info("batch index %d", workload.batchIndex)
	o.Log.Info("batch size %d", workload.batchSize)
	o.Log.Info("remainder size %d", workload.remainder)

	wg.Add(workload.batchSize)
	for i := 0; i < workload.count; i++ {
		o.Log.Info(fmt.Sprintf("starting batch %d ", i))
		for x := 0; x < workload.batchSize; x++ {
			index := (i * workload.batchSize) + x
			irs, err := customImageParser(workload.copyImages[index].Image)
			if err != nil {
				o.Log.Error("%v", err)
				continue
			}
			// ignore the failure as it will be picked up in the Run
			err = os.MkdirAll(strings.Split(opts.Destination, ":")[1]+"/"+irs.Namespace, 0750)
			if err != nil {
				return err
			}
			src := dockerProtocol + workload.copyImages[index].Image
			dest := opts.Destination + "/" + irs.Namespace + "/" + irs.Component

			o.Log.Debug("source %s ", workload.copyImages[index].Image)
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
		if workload.count > 1 {
			wg.Add(BATCH_SIZE)
		}
		if len(errArray) > 0 {
			for _, err := range errArray {
				o.Log.Error(" errArray %v", err)
			}
			return fmt.Errorf("error in batch - refer to console logs")
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
