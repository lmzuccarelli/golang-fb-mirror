package batch

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/lmzuccarelli/golang-oci-mirror/pkg/api/v1alpha3"
	clog "github.com/lmzuccarelli/golang-oci-mirror/pkg/log"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/manifest"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/mirror"
)

const (
	BATCH_SIZE int    = 8
	logFile    string = "logs/worker-{batch}.log"
)

type BatchInterface interface {
	Worker(ctx context.Context, images []string, opts mirror.CopyOptions) error
}

func New(log clog.PluggableLoggerInterface,
	mirror mirror.MirrorInterface,
	manifest manifest.ManifestInterface,
) BatchInterface {
	return &Batch{Log: log, Mirror: mirror, Manifest: manifest}
}

type Batch struct {
	Log      clog.PluggableLoggerInterface
	Mirror   mirror.MirrorInterface
	Manifest manifest.ManifestInterface
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

func (o *Batch) Worker(ctx context.Context, images []string, opts mirror.CopyOptions) error {

	var errArray []error
	var wg sync.WaitGroup
	var err error

	var b *BatchSchema
	imgs := len(images)
	if imgs < BATCH_SIZE {
		b = &BatchSchema{Items: imgs, Count: 1, BatchSize: imgs, BatchIndex: 0, Remainder: 0}
	} else {
		b = &BatchSchema{Items: imgs, Count: (imgs / BATCH_SIZE), BatchSize: BATCH_SIZE, Remainder: (imgs % BATCH_SIZE)}
	}

	o.Log.Info("images to mirror %d ", b.Items)
	o.Log.Info("batch count %d ", b.Count)
	o.Log.Info("batch index %d ", b.BatchIndex)
	o.Log.Info("batch size %d ", b.BatchSize)
	o.Log.Info("remainder size %d ", b.Remainder)
	o.Log.Info("image type %s ", opts.ImageType)

	f := make([]*os.File, b.Count)
	//f, err := make([]os.File)
	// prepare batching
	wg.Add(b.BatchSize)
	for i := 0; i < b.Count; i++ {
		// create a log file for each batch
		f[i], err = os.Create(strings.Replace(logFile, "{batch}", strconv.Itoa(i), -1))
		if err != nil {
			o.Log.Error("[Worker] %v", err)
		}
		writer := bufio.NewWriter(f[i])
		o.Log.Info(fmt.Sprintf("starting batch %d ", i))
		for x := 0; x < b.BatchSize; x++ {
			index := (i * b.BatchSize) + x
			hld := strings.Split(images[index], "*")
			if len(hld) == 0 {
				return fmt.Errorf("the source and destination selector is missing")
			}
			o.Log.Debug("destination %s ", hld[1])
			go func(ctx context.Context, src, dest string, opts *mirror.CopyOptions, writer bufio.Writer) {
				defer wg.Done()
				err := o.Mirror.Run(ctx, src, dest, opts, writer)
				if err != nil {
					errArray = append(errArray, err)
				}
			}(ctx, hld[0], hld[1], &opts, *writer)
		}
		wg.Wait()
		// rather than use defer Close we intentianally close the log files
		for _, f := range f {
			f.Close()
		}
		o.Log.Info("completed batch %d", i)
		if b.Count > 1 {
			wg.Add(BATCH_SIZE)
		}
		if len(errArray) > 0 {
			for _, err := range errArray {
				o.Log.Error("[Worker] errArray %v", err)
			}
			return fmt.Errorf("[Worker] error in batch - refer to console logs")
		}
	}
	if b.Remainder > 0 {
		// one level of simple recursion
		i := b.Count * BATCH_SIZE
		o.Log.Info("executing remainder ")
		err := o.Worker(ctx, images[i:], opts)
		if err != nil {
			return err
		}
		// output the logs to console
		consoleLogFromFile(o.Log)
	}
	return nil
}

func consoleLogFromFile(log clog.PluggableLoggerInterface) {
	dir, _ := os.ReadDir("logs")
	for _, f := range dir {
		if strings.Contains(f.Name(), "worker") {
			log.Debug("[batch] %s ", f.Name())
			data, _ := os.ReadFile("logs/" + f.Name())
			lines := strings.Split(string(data), "\n")
			for _, s := range lines {
				if len(s) > 0 {
					// clean the line
					log.Debug("%s ", strings.ToLower(s))
				}
			}
		}
		fmt.Println(" ")
	}
}
