package ci

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const maxIntegerAPKMetadata = 16 << 20

type integerAPKIdentity struct {
	architecture IntegerArchitecture
	name         string
	digest       string
}

func inspectIntegerAPK(path string) (integerAPKIdentity, error) {
	file, err := os.Open(path)
	if err != nil {
		return integerAPKIdentity{}, fmt.Errorf("open APK %q: %w", path, err)
	}
	defer file.Close()
	hash := sha256.New()
	reader := bufio.NewReader(io.TeeReader(file, hash))
	var identity integerAPKIdentity
	for stream := 0; stream < 3 && identity.name == ""; stream++ {
		gzipReader, err := gzip.NewReader(reader)
		if err != nil {
			return integerAPKIdentity{}, fmt.Errorf("open APK %q gzip stream %d: %w", path, stream, err)
		}
		gzipReader.Multistream(false)
		identity, err = inspectIntegerAPKStream(tar.NewReader(gzipReader))
		closeErr := gzipReader.Close()
		if err != nil {
			return integerAPKIdentity{}, fmt.Errorf("inspect APK %q stream %d: %w", path, stream, err)
		}
		if closeErr != nil {
			return integerAPKIdentity{}, fmt.Errorf("close APK %q stream %d: %w", path, stream, closeErr)
		}
	}
	if identity.name == "" {
		return integerAPKIdentity{}, fmt.Errorf("%w: APK %q has no .PKGINFO", ErrIntegerBatchPlan, path)
	}
	if _, err := io.Copy(io.Discard, reader); err != nil {
		return integerAPKIdentity{}, fmt.Errorf("hash APK %q: %w", path, err)
	}
	identity.digest = "sha256:" + hex.EncodeToString(hash.Sum(nil))
	return identity, nil
}

func inspectIntegerAPKStream(reader *tar.Reader) (integerAPKIdentity, error) {
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return integerAPKIdentity{}, nil
		}
		if err != nil {
			return integerAPKIdentity{}, fmt.Errorf("read APK tar header: %w", err)
		}
		if header.Name != ".PKGINFO" {
			continue
		}
		if header.Size < 0 || header.Size > maxIntegerAPKMetadata {
			return integerAPKIdentity{}, fmt.Errorf("%w: invalid .PKGINFO size", ErrIntegerBatchPlan)
		}
		contents, err := io.ReadAll(io.LimitReader(reader, maxIntegerAPKMetadata+1))
		if err != nil {
			return integerAPKIdentity{}, fmt.Errorf("read .PKGINFO: %w", err)
		}
		return parseIntegerPackageInfo(string(contents))
	}
}

func parseIntegerPackageInfo(contents string) (integerAPKIdentity, error) {
	var identity integerAPKIdentity
	for line := range strings.SplitSeq(contents, "\n") {
		key, value, ok := strings.Cut(line, " = ")
		if !ok {
			continue
		}
		switch key {
		case "pkgname":
			identity.name = strings.TrimSpace(value)
		case "arch":
			identity.architecture = IntegerArchitecture(strings.TrimSpace(value))
		}
	}
	if !integerPackageNamePattern.MatchString(identity.name) || !validIntegerArchitecture(identity.architecture) {
		return integerAPKIdentity{}, fmt.Errorf("%w: invalid APK package identity %q/%q", ErrIntegerBatchPlan, identity.architecture, identity.name)
	}
	return identity, nil
}
