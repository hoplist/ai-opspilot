//go:build !windows

package main

import "syscall"

func filesystemUsage(path string) (uint64, uint64, uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, 0, err
	}
	blockSize := uint64(stat.Bsize)
	total := stat.Blocks * blockSize
	free := stat.Bfree * blockSize
	avail := stat.Bavail * blockSize
	return total, free, avail, nil
}
