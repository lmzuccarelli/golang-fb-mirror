package schema

import (
	"time"
)

type ReleaseSchema struct {
	Kind       string `json:"kind"`
	APIVersion string `json:"apiVersion"`
	Metadata   struct {
		Name              string    `json:"name"`
		CreationTimestamp time.Time `json:"creationTimestamp"`
		Annotations       struct {
			ReleaseOpenshiftIoFromImageStream string `json:"release.openshift.io/from-image-stream"`
		} `json:"annotations"`
	} `json:"metadata"`
	Spec struct {
		LookupPolicy struct {
			Local bool `json:"local"`
		} `json:"lookupPolicy"`
		Tags []struct {
			Name string `json:"name"`
			From struct {
				Kind string `json:"kind"`
				Name string `json:"name"`
			} `json:"from"`
			Generation   interface{} `json:"generation"`
			ImportPolicy struct {
			} `json:"importPolicy"`
			ReferencePolicy struct {
				Type string `json:"type"`
			} `json:"referencePolicy"`
			Annotations struct {
				IoOpenshiftBuildCommitID            string `json:"io.openshift.build.commit.id"`
				IoOpenshiftBuildCommitRef           string `json:"io.openshift.build.commit.ref"`
				IoOpenshiftBuildSourceLocation      string `json:"io.openshift.build.source-location"`
				IoOpenshiftBuildVersionDisplayNames string `json:"io.openshift.build.version-display-names"`
				IoOpenshiftBuildVersions            string `json:"io.openshift.build.versions"`
			} `json:"annotations,omitempty"`
		} `json:"tags"`
	} `json:"spec"`
	Status struct {
		DockerImageRepository string `json:"dockerImageRepository"`
	} `json:"status"`
}

type OCISchema struct {
	SchemaVersion int    `json:"schemaVersion"`
	MediaType     string `json:"mediaType"`
	Manifests     []struct {
		MediaType string `json:"mediaType"`
		Digest    string `json:"digest"`
		Size      int    `json:"size"`
	} `json:"manifests"`
	Config struct {
		MediaType string `json:"mediaType"`
		Digest    string `json:"digest"`
		Size      int    `json:"size"`
	} `json:"config"`
	Layers []struct {
		MediaType string `json:"mediaType"`
		Digest    string `json:"digest"`
		Size      int    `json:"size"`
	} `json:"layers"`
}
