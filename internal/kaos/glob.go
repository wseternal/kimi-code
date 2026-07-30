// Package kaos provides advanced glob with symlink cycle detection (Gap #65).
package kaos

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
)

// inodeKey uniquely identifies a file by device and inode number.
type inodeKey struct {
	Dev uint64
	Ino uint64
}

// getInodeKey extracts the (dev, ino) pair from a FileInfo.
func getInodeKey(info os.FileInfo) (inodeKey, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return inodeKey{}, false
	}
	return inodeKey{
		Dev: uint64(stat.Dev),
		Ino: stat.Ino,
	}, true
}

// GlobOptions configures glob behavior.
type GlobOptions struct {
	// MaxDepth limits recursion depth (0 = unlimited).
	MaxDepth int
	// FollowSymlinks follows symbolic links (with cycle detection).
	FollowSymlinks bool
	// IncludeHidden includes dotfiles/dotdirs.
	IncludeHidden bool
	// MaxResults limits the number of results (0 = unlimited).
	MaxResults int
}

// DefaultGlobOptions returns sensible defaults.
func DefaultGlobOptions() GlobOptions {
	return GlobOptions{
		MaxDepth:       0,
		FollowSymlinks: false,
		IncludeHidden:  false,
		MaxResults:     10000,
	}
}

// GlobResult holds the result of a glob operation.
type GlobResult struct {
	Matches  []string
	Truncated bool // true if MaxResults was hit
}

// AdvancedGlob performs recursive glob matching with symlink cycle detection.
func AdvancedGlob(root, pattern string, opts GlobOptions) (*GlobResult, error) {
	visited := &sync.Map{}
	result := &GlobResult{}
	maxResults := opts.MaxResults
	if maxResults <= 0 {
		maxResults = 10000
	}

	err := walkGlob(root, pattern, opts, 0, visited, result, maxResults)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func walkGlob(root, pattern string, opts GlobOptions, depth int, visited *sync.Map, result *GlobResult, maxResults int) error {
	if opts.MaxDepth > 0 && depth > opts.MaxDepth {
		return nil
	}
	if len(result.Matches) >= maxResults {
		result.Truncated = true
		return nil
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil // skip unreadable dirs
	}

	for _, entry := range entries {
		if len(result.Matches) >= maxResults {
			result.Truncated = true
			return nil
		}

		name := entry.Name()
		if !opts.IncludeHidden && strings.HasPrefix(name, ".") {
			continue
		}

		fullPath := filepath.Join(root, name)
		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Handle symlinks
		if info.Mode()&os.ModeSymlink != 0 && opts.FollowSymlinks {
			realInfo, err := os.Stat(fullPath)
			if err != nil {
				continue
			}
			if realInfo.IsDir() {
				key, ok := getInodeKey(realInfo)
				if ok {
					if _, loaded := visited.LoadOrStore(key, true); loaded {
						continue // cycle detected
					}
				}
				if err := walkGlob(fullPath, pattern, opts, depth+1, visited, result, maxResults); err != nil {
					return err
				}
			} else {
				if matched, _ := filepath.Match(pattern, name); matched {
					result.Matches = append(result.Matches, fullPath)
				}
			}
			continue
		}

		if info.IsDir() {
			if err := walkGlob(fullPath, pattern, opts, depth+1, visited, result, maxResults); err != nil {
				return err
			}
		} else {
			if matched, _ := filepath.Match(pattern, name); matched {
				result.Matches = append(result.Matches, fullPath)
			}
		}
	}
	return nil
}

// IsSymlinkCycle detects if a path leads to a symlink cycle.
func IsSymlinkCycle(path string, visited *sync.Map) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	key, ok := getInodeKey(info)
	if !ok {
		return false
	}
	_, loaded := visited.LoadOrStore(key, true)
	return loaded
}

// GlobPatterns splits a compound pattern (e.g., "**/*.{go,ts}") into simple patterns.
func GlobPatterns(pattern string) []string {
	// Handle brace expansion: {a,b,c}
	if idx := strings.Index(pattern, "{"); idx >= 0 {
		if end := strings.Index(pattern[idx:], "}"); end >= 0 {
			end += idx
			prefix := pattern[:idx]
			suffix := pattern[end+1:]
			alts := strings.Split(pattern[idx+1:end], ",")
			var result []string
			for _, alt := range alts {
				result = append(result, prefix+strings.TrimSpace(alt)+suffix)
			}
			return result
		}
	}
	return []string{pattern}
}
