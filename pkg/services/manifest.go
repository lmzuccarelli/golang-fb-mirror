package services

import (
	"encoding/json"
	"os"

	"github.com/lmzuccarelli/golang-oci-mirror/pkg/schema"
)

const (
	index string = "index.json"
)

func GetImageIndex(dir string) (*schema.OCISchema, error) {
	var oci *schema.OCISchema
	indx, err := os.ReadFile(dir + "/" + index)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(indx, &oci)
	if err != nil {
		return nil, err
	}
	return oci, nil
}

func GetImageManifest(file string) (*schema.OCISchema, error) {
	var oci *schema.OCISchema
	manifest, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(manifest, &oci)
	if err != nil {
		return nil, err
	}
	return oci, nil
}
