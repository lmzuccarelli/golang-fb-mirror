# Overview

## POC implementation of oci format for mirroring

This is a simpler implementation of the functionality found in oc-mirror

As mentioned its still very raw (hence POC) but has the following functionality

All image formats are in OCI

- cincinnati client (release and channel versioning)
- release downloads to disk from registry
- operator downloads to disk from registry
- additionalImages to disk from registry

- release upload to registry from disk
- operator upload to registry from disk
- additionalImages to registry from disk

At this point in time the problems/lack of features are 
- mirrorTomirror functionality (not yet tested)
- limited tests
- detached api's - need to implement for upstream api changes

