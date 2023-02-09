package services

import (
	"context"
)

func (o *Executor) ExecuteOperators(ctx context.Context) error {

	allRelatedImages, err := o.OperatorImageCollector(ctx)
	if err != nil {
		return err
	}
	o.Log.Info("total images to copy %d ", len(allRelatedImages))

	var batch *BatchSchema
	images := len(allRelatedImages)
	if images < BATCH_SIZE {
		batch = &BatchSchema{items: images, count: 1, batchSize: images, batchIndex: 0, remainder: 0}
	} else {
		batch = &BatchSchema{items: images, count: (images / BATCH_SIZE), batchSize: BATCH_SIZE, remainder: (images % BATCH_SIZE)}
	}
	batch.copyImages = allRelatedImages

	// call the batch executioner
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
	return nil
}
