package services

const (
	workingDir                  string = "working-dir/"
	dockerProtocol              string = "docker://"
	ociProtocol                 string = "oci:"
	releaseImageDir             string = "release-images"
	operatorImageDir            string = "operator-images"
	releaseImageExtractDir      string = "hold-release"
	releaseManifests            string = "release-manifests"
	imageReferences             string = "image-references"
	releaseImageExtractFullPath string = releaseImageExtractDir + "/" + releaseManifests + "/" + imageReferences
	blobsDir                    string = "/blobs/sha256/"
	BATCH_SIZE                  int    = 8
	diskToMirror                string = "diskToMirror"
	mirrorToDisk                string = "mirrorToDisk"
)
