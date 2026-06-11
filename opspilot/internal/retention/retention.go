package retention

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Policy struct {
	MaxItems  int
	MaxAge    time.Duration
	MaxBytes  int64
	Extension []string
}

func CleanupDir(dir string, policy Policy) error {
	dir = strings.TrimSpace(dir)
	if dir == "" || !policy.enabled() {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	files := []fileInfo{}
	for _, entry := range entries {
		if entry.IsDir() || !matchesExtension(entry.Name(), policy.Extension) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, fileInfo{
			path:    filepath.Join(dir, entry.Name()),
			modTime: info.ModTime(),
			size:    info.Size(),
		})
	}
	cutoff := time.Time{}
	if policy.MaxAge > 0 {
		cutoff = time.Now().Add(-policy.MaxAge)
	}
	kept := files[:0]
	for _, file := range files {
		if !cutoff.IsZero() && file.modTime.Before(cutoff) {
			if err := os.Remove(file.path); err != nil && !os.IsNotExist(err) {
				return err
			}
			continue
		}
		kept = append(kept, file)
	}
	sort.SliceStable(kept, func(i, j int) bool {
		return kept[i].modTime.After(kept[j].modTime)
	})
	if policy.MaxItems > 0 && len(kept) > policy.MaxItems {
		for _, file := range kept[policy.MaxItems:] {
			if err := os.Remove(file.path); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		kept = kept[:policy.MaxItems]
	}
	if policy.MaxBytes <= 0 {
		return nil
	}
	var total int64
	for _, file := range kept {
		total += file.size
	}
	for i := len(kept) - 1; i >= 0 && total > policy.MaxBytes; i-- {
		if err := os.Remove(kept[i].path); err != nil && !os.IsNotExist(err) {
			return err
		}
		total -= kept[i].size
	}
	return nil
}

type fileInfo struct {
	path    string
	modTime time.Time
	size    int64
}

func (p Policy) enabled() bool {
	return p.MaxItems > 0 || p.MaxAge > 0 || p.MaxBytes > 0
}

func matchesExtension(name string, extensions []string) bool {
	if len(extensions) == 0 {
		return true
	}
	for _, extension := range extensions {
		if strings.HasSuffix(name, extension) {
			return true
		}
	}
	return false
}
