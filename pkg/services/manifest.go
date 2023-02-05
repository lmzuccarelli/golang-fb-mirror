package services

import (
	"encoding/json"
	"os"

	"github.com/lmzuccarelli/golang-oci-mirror/pkg/api/v1alpha3"
)

const (
	index string = "index.json"
)

// GetImageIndex - used to get the oci index.json
func GetImageIndex(dir string) (*v1alpha3.OCISchema, error) {
	var oci *v1alpha3.OCISchema
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

// GetImageManifest used to ge the manifest in the oci blobs/sha254
// directory - found in index.json
func GetImageManifest(file string) (*v1alpha3.OCISchema, error) {
	var oci *v1alpha3.OCISchema
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

// GetOperatorConfig used to parse the operator json
func GetOperatorConfig(file string) (*v1alpha3.OperatorConfigSchema, error) {
	var ocs *v1alpha3.OperatorConfigSchema
	manifest, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(manifest, &ocs)
	if err != nil {
		return nil, err
	}
	return ocs, nil
}
