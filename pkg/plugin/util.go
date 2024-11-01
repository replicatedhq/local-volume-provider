package plugin

import (
	"io/fs"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const defaultRoot = "/var/velero-local-volume-provider"

// getRoot returns the internal mount point of the Velero container for the local volumes.
func getRoot() string {
	root := os.Getenv("VOLUME_ROOT")
	if root != "" {
		return root
	}

	return defaultRoot
}

// getSubDirectoryLayout returns the default subdirectories Velero expects in the an ObjectStore.
// https://github.com/vmware-tanzu/velero/blob/eefd12b3e48323ec59f88ef5bbbf8251fad04a26/pkg/persistence/object_store_layout.go
func getSubDirectoryLayout() []string {
	return []string{
		"backups",
		"restores",
		"restic",
		"metadata",
		"plugins",
	}
}

func sliceContainsString(list []string, s string) bool {
	for _, v := range list {
		if strings.Contains(v, s) {
			return true
		}
	}
	return false
}

// isWritable compared the unix files permissions from the info object to the currently running user
// and returns truthy if the location is writeable.
func isWriteable(log *logrus.Entry, info fs.FileInfo) bool {

	// Wide open
	log.Debugf("user: %d", info.Mode().Perm()&(1<<1))
	if info.Mode().Perm()&(1<<1) > 0 {
		return true
	}

	stat := info.Sys().(*syscall.Stat_t)
	log.Debugf("Owner Uid: %d", int(stat.Uid))
	log.Debugf("Detected Uid: %d", os.Geteuid())

	// Writable by user
	if info.Mode().Perm()&(1<<7) > 0 && os.Geteuid() == int(stat.Uid) {
		return true
	}

	log.Debugf("Owner Gid: %d", int(stat.Gid))
	log.Debugf("Detected Gid: %d", os.Getegid())

	// Writable by group
	if info.Mode().Perm()&(1<<4) > 0 && os.Getegid() == int(stat.Gid) || os.Getegid() == 0 {
		return true
	}

	return false
}

// StringToIntPointer converts a string to a pointer to an int.
func StringToIntPointer(x string) (*int64, error) {
	var xout int64
	xout, err := strconv.ParseInt(x, 0, 64)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse int")
	}
	return &xout, nil
}
