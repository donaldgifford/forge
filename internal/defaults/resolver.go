// Package defaults resolves layered _defaults/ directory inheritance for forge blueprints.
package defaults

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SourceLayer identifies where a file originates in the inheritance chain.
type SourceLayer int

const (
	// LayerRegistryDefault is a file from the root /_defaults/ directory.
	LayerRegistryDefault SourceLayer = iota

	// LayerCategoryDefault is a file from an intermediate _defaults/ directory (e.g., /go/_defaults/).
	LayerCategoryDefault

	// LayerBlueprint is a file from the blueprint directory itself.
	LayerBlueprint
)

// String returns a human-readable name for the source layer.
func (s SourceLayer) String() string {
	switch s {
	case LayerRegistryDefault:
		return "registry-default"
	case LayerCategoryDefault:
		return "category-default"
	case LayerBlueprint:
		return "blueprint"
	default:
		return "unknown"
	}
}

const defaultsDirName = "_defaults"

// FileEntry represents a single file in the resolved file set.
type FileEntry struct {
	// AbsPath is the absolute path to the source file on disk.
	AbsPath string

	// RelPath is the relative path the file will have in the output.
	RelPath string

	// SourceLayer tracks where this file came from in the inheritance chain.
	SourceLayer SourceLayer

	// IsTemplate is true if the file ends with .tmpl.
	IsTemplate bool
}

// FileSet is an ordered collection of files from the resolved inheritance chain.
type FileSet struct {
	files map[string]*FileEntry
	order []string
}

// NewFileSet creates an empty FileSet.
func NewFileSet() *FileSet {
	return &FileSet{
		files: make(map[string]*FileEntry),
	}
}

// Add adds or replaces a file entry keyed by relative path.
func (fs *FileSet) Add(entry *FileEntry) {
	if _, exists := fs.files[entry.RelPath]; !exists {
		fs.order = append(fs.order, entry.RelPath)
	}

	fs.files[entry.RelPath] = entry
}

// Remove deletes a file from the set by relative path.
func (fs *FileSet) Remove(relPath string) {
	delete(fs.files, relPath)
}

// Get returns a file entry by relative path, or nil if not found.
func (fs *FileSet) Get(relPath string) *FileEntry {
	return fs.files[relPath]
}

// Entries returns all file entries in insertion order.
func (fs *FileSet) Entries() []*FileEntry {
	entries := make([]*FileEntry, 0, len(fs.files))

	for _, key := range fs.order {
		if entry, ok := fs.files[key]; ok {
			entries = append(entries, entry)
		}
	}

	return entries
}

// Len returns the number of files in the set.
func (fs *FileSet) Len() int {
	return len(fs.files)
}

// Resolve walks the registry directory tree and merges the layered _defaults/
// directories with the blueprint's own files.
//
// Walk order (last wins):
//  1. /<registryRoot>/_defaults/ → LayerRegistryDefault
//  2. /<registryRoot>/<category>/_defaults/ for each path segment → LayerCategoryDefault
//  3. /<registryRoot>/<blueprintPath>/ → LayerBlueprint
//
// Files listed in exclusions are removed from the result.
func Resolve(registryRoot, blueprintPath string, exclusions []string) (*FileSet, error) {
	fs := NewFileSet()

	// 1. Root _defaults/
	rootDefaults := filepath.Join(registryRoot, defaultsDirName)
	if err := collectFiles(rootDefaults, fs, LayerRegistryDefault); err != nil {
		return nil, fmt.Errorf("collecting root defaults: %w", err)
	}

	// 2. Category _defaults/ directories between root and blueprint.
	segments := strings.Split(blueprintPath, "/")
	for i := range len(segments) - 1 {
		categoryPath := filepath.Join(registryRoot, filepath.Join(segments[:i+1]...), defaultsDirName)
		if err := collectFiles(categoryPath, fs, LayerCategoryDefault); err != nil {
			return nil, fmt.Errorf("collecting category defaults at %s: %w", categoryPath, err)
		}
	}

	// 3. Blueprint directory itself.
	bpDir := filepath.Join(registryRoot, blueprintPath)
	if err := collectFiles(bpDir, fs, LayerBlueprint); err != nil {
		return nil, fmt.Errorf("collecting blueprint files: %w", err)
	}

	// 4. Apply exclusions.
	for _, excl := range exclusions {
		fs.Remove(excl)
	}

	return fs, nil
}

// collectFiles walks a directory and adds all regular files to the FileSet.
// The _defaults directory name is skipped when collecting blueprint files.
// The blueprint.yaml file is also skipped as it's metadata, not output content.
func collectFiles(dir string, fs *FileSet, layer SourceLayer) error {
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("stat %s: %w", dir, err)
	}

	if !info.IsDir() {
		return nil
	}

	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip _defaults subdirectories (they belong to child categories),
		// but not the root directory itself when we're walking a _defaults dir.
		if info.IsDir() && info.Name() == defaultsDirName && path != dir {
			return filepath.SkipDir
		}

		if info.IsDir() {
			return nil
		}

		// Skip blueprint.yaml — it's config, not output content.
		if info.Name() == "blueprint.yaml" {
			return nil
		}

		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return fmt.Errorf("computing relative path for %s: %w", path, err)
		}

		fs.Add(&FileEntry{
			AbsPath:     path,
			RelPath:     relPath,
			SourceLayer: layer,
			IsTemplate:  strings.HasSuffix(path, ".tmpl"),
		})

		return nil
	})
}
