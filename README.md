# Overview

## POC implementation of oci format for mirroring

This is a simpler implementation of the functionality found in oc-mirror

As mentioned its still very raw (hence POC) but has the following functionality

All image formats are in OCI

- cincinnati client (release and channel versioning)
- release downloads to disk from registry
- opertaor downloads to disk from registry
- additionalImages to disk from registry

- release upload to registry from disk
- operator upload to registry from disk
- additionalImages to registry from disk

At this point in time there is no 
- mirrorTomirror functionality
- no pruning
- limited tests
- incremental runs 
- no diff and pruning from previos runs

