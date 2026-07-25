package main

import (
	"errors"
	"fmt"
)

const (
	binaryName    = "verity"
	checksumName  = "verity.sha256"
	buildJSONName = "build.json"
	buildVersion  = "2.0.0"

	artifactNamePrefix      = "verity-linux-amd64-"
	metadataPackage         = "github.com/verity-org/verity/internal/buildmetadata"
	metadataLDFlagsContract = "-ldflags=-X buildmetadata.version=2.0.0 -X buildmetadata.sourceSHA=<source_sha> -X buildmetadata.buildKey=<build_key> -X buildmetadata.buildFlags=<build_flags>"
)

var (
	errUntrustedArtifact  = errors.New("untrusted Verity artifact")
	errMissingCommand     = errors.New("missing command")
	errUnsupportedCommand = errors.New("unsupported command")
	canonicalBuildFlags   = []string{"-buildvcs=true", "-trimpath"}
	runtimeBuildFlags     = []string{"-buildmode=exe", "-buildvcs=true", "-compiler=gc", "-trimpath"}
)

type artifactIdentity struct {
	SourceSHA string
	BuildKey  string
}

func (identity artifactIdentity) valid() bool {
	return lowerHex(identity.SourceSHA, 40) && lowerHex(identity.BuildKey, 64)
}

func lowerHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func untrusted(reason string) error {
	return fmt.Errorf("%w: %s", errUntrustedArtifact, reason)
}
