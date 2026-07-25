package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type goListModule struct {
	Main bool
}

type goListPackage struct {
	Dir          string
	GoFiles      []string
	CgoFiles     []string
	CFiles       []string
	CXXFiles     []string
	MFiles       []string
	HFiles       []string
	FFiles       []string
	SFiles       []string
	SwigFiles    []string
	SwigCXXFiles []string
	SysoFiles    []string
	EmbedFiles   []string
	TestGoFiles  []string
	XTestGoFiles []string
	Module       *goListModule
}

type productionFileSnapshot struct {
	pathInfo os.FileInfo
	fileInfo os.FileInfo
	resolved string
}

func productionDependencyFiles(ctx context.Context, root string) ([]string, error) {
	output, err := goCommandOutput(ctx, root, "list", "-deps", "-json", ".")
	if err != nil {
		return nil, fmt.Errorf("list production Go dependencies: %w", err)
	}
	return decodeProductionDependencyFiles(root, strings.NewReader(output))
}

func decodeProductionDependencyFiles(root string, input io.Reader) ([]string, error) {
	files := map[string]struct{}{
		"go.mod":    {},
		"go.sum":    {},
		"mise.lock": {},
		"mise.toml": {},
	}
	decoder := json.NewDecoder(input)
	for {
		var pkg goListPackage
		if err := decoder.Decode(&pkg); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("decode production Go dependencies: %w", err)
		}
		if pkg.Module == nil || !pkg.Module.Main {
			continue
		}
		classes := [][]string{
			pkg.GoFiles, pkg.CgoFiles, pkg.CFiles, pkg.CXXFiles, pkg.MFiles, pkg.HFiles,
			pkg.FFiles, pkg.SFiles, pkg.SwigFiles, pkg.SwigCXXFiles, pkg.SysoFiles, pkg.EmbedFiles,
		}
		for _, names := range classes {
			for _, name := range names {
				relative, err := productionRelativePath(root, pkg.Dir, name)
				if err != nil {
					return nil, err
				}
				if _, duplicate := files[relative]; duplicate {
					return nil, untrusted("duplicate production build input")
				}
				files[relative] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(files))
	for path := range files {
		result = append(result, path)
	}
	sort.Strings(result)
	return result, nil
}

func productionRelativePath(root, directory, name string) (string, error) {
	if !filepath.IsAbs(directory) || filepath.Clean(directory) != directory || !filepath.IsLocal(name) || filepath.Clean(name) != name {
		return "", untrusted("production dependency path")
	}
	relative, err := filepath.Rel(root, filepath.Join(directory, name))
	if err != nil || relative == "." || !filepath.IsLocal(relative) {
		return "", untrusted("production dependency path")
	}
	return canonicalProductionRelativePath(filepath.ToSlash(relative))
}

func canonicalProductionRelativePath(relative string) (string, error) {
	local := filepath.FromSlash(relative)
	if relative == "" || relative == "." || strings.Contains(relative, "\\") || !filepath.IsLocal(local) || filepath.ToSlash(filepath.Clean(local)) != relative {
		return "", untrusted("production dependency path")
	}
	if strings.HasSuffix(relative, "_test.go") || relative == "test" || strings.HasPrefix(relative, "test/") {
		return "", untrusted("test-only production dependency path")
	}
	return relative, nil
}

func readStableProductionFile(path string) (data []byte, err error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect production build input: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, untrusted("production build input file type")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open production build input: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close production build input: %w", closeErr))
		}
	}()
	return readOpenedProductionFile(path, file)
}

func readOpenedProductionFile(path string, file *os.File) ([]byte, error) {
	before, err := inspectOpenedProductionFile(path, file)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read production build input: %w", err)
	}
	after, err := inspectOpenedProductionFile(path, file)
	if err != nil {
		return nil, err
	}
	if !sameProductionFile(before, after) || before.fileInfo.Size() != int64(len(data)) {
		return nil, untrusted("unstable production build input")
	}
	return data, nil
}

func inspectOpenedProductionFile(path string, file *os.File) (productionFileSnapshot, error) {
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return productionFileSnapshot{}, fmt.Errorf("inspect production build input: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return productionFileSnapshot{}, fmt.Errorf("resolve production build input: %w", err)
	}
	fileInfo, err := file.Stat()
	if err != nil {
		return productionFileSnapshot{}, fmt.Errorf("inspect opened production build input: %w", err)
	}
	if resolved != filepath.Clean(path) || !pathInfo.Mode().IsRegular() || !fileInfo.Mode().IsRegular() {
		return productionFileSnapshot{}, untrusted("production build input file type")
	}
	if !os.SameFile(pathInfo, fileInfo) {
		return productionFileSnapshot{}, untrusted("unstable production build input")
	}
	return productionFileSnapshot{pathInfo: pathInfo, fileInfo: fileInfo, resolved: resolved}, nil
}

func sameProductionFile(before, after productionFileSnapshot) bool {
	return before.resolved == after.resolved &&
		os.SameFile(before.pathInfo, after.pathInfo) &&
		os.SameFile(before.fileInfo, after.fileInfo) &&
		before.fileInfo.Mode() == after.fileInfo.Mode() &&
		before.fileInfo.Size() == after.fileInfo.Size() &&
		before.fileInfo.ModTime().Equal(after.fileInfo.ModTime())
}
