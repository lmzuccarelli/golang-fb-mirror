package sequence

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/api/v1alpha2"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/config"
	"gopkg.in/yaml.v2"
)

const (
	metadataFile string = ".metadata.toml"
)

func CalculateDiff(dir string, metadata SequenceSchema, cfg v1alpha2.ImageSetConfiguration) error {

	toDelete := make(map[string]bool)
	prevConfig, res, err := CheckDiff(dir, metadata, cfg)
	fmt.Println("DEBUG LMZ ", res)
	fmt.Println("DEBUG LMZ ", prevConfig)
	if err != nil {
		// TODO: update with logger
		return fmt.Errorf("could not calculate diff %v ", err)
	}
	if res {
		// lets get the differences
		// iterate through each object find the differences
		// if the image object is found in the previous config
		// then its just an add we don't need to prune
		// if its not found wecreate a map to delete
		for _, r := range cfg.Mirror.Platform.Channels {
			for _, pr := range prevConfig.Mirror.Platform.Channels {
				if reflect.DeepEqual(r, pr) {
					toDelete[r.Name] = true
				}
			}
			toDelete[r.Name] = false
		}
		for _, op := range cfg.Mirror.Operators {
			for _, prOp := range prevConfig.Mirror.Operators {
				if reflect.DeepEqual(op, prOp) {
					toDelete[prOp.Catalog] = true
				}
			}
			toDelete[op.Catalog] = false
		}
		for _, ai := range cfg.Mirror.AdditionalImages {
			for _, prAi := range prevConfig.Mirror.AdditionalImages {
				if reflect.DeepEqual(ai, prAi) {
					toDelete[prAi.Name] = true
				}
			}
			toDelete[ai.Name] = false
		}
	}
	fmt.Println("DEBUG LMZ ", toDelete)
	return nil
}

func CheckDiff(dir string, metadata SequenceSchema, cfg v1alpha2.ImageSetConfiguration) (v1alpha2.ImageSetConfiguration, bool, error) {
	var isc string

	for _, item := range metadata.Sequence.Item {
		if item.Current {
			isc = item.Imagesetconfig
		}
	}

	if len(isc) == 0 {
		return v1alpha2.ImageSetConfiguration{}, false, fmt.Errorf("no valid previous ImageSetConfiguration file found")
	}

	prevCfg, err := config.ReadConfig(dir + "/" + isc)
	if err != nil {
		return v1alpha2.ImageSetConfiguration{}, false, err
	}

	if !reflect.DeepEqual(cfg, prevCfg) {
		return prevCfg, true, nil
	}

	return v1alpha2.ImageSetConfiguration{}, false, nil
}

func ReadMetaData(dir string) (SequenceSchema, error) {
	var schema SequenceSchema
	if _, err := toml.DecodeFile(dir+"/"+metadataFile, &schema); err != nil {
		return SequenceSchema{}, err
	}
	return schema, nil
}

func WriteMetadata(dir string, sch SequenceSchema, cfg v1alpha2.ImageSetConfiguration) error {

	for i := range sch.Sequence.Item {
		sch.Sequence.Item[i].Current = false
	}

	newItem := &Item{
		Value:          len(sch.Sequence.Item),
		Current:        true,
		Imagesetconfig: dir + "/.imagesetconfig-" + strconv.Itoa(len(sch.Sequence.Item)) + ".yaml",
		Timestamp:      time.Now().Unix(),
	}

	sch.Sequence.Item = append(sch.Sequence.Item, *newItem)
	f, err := os.Create(dir + "/" + metadataFile)
	if err != nil {
		// failed to create/open the file
		return err
	}
	if err := toml.NewEncoder(f).Encode(sch); err != nil {
		// failed to encode
		return err
	}
	if err := f.Close(); err != nil {
		// failed to close the file
		return err
	}

	data, err := yaml.Marshal(&sch)
	if err != nil {
		// failed to create/open the file
		return err
	}
	if err != nil {
		return err
	}

	err = os.WriteFile(newItem.Imagesetconfig, data, 0644)
	if err != nil {
		return err
	}

	return nil
}
