package plugin

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ensureFilesystem checks that the filesystem is ready for use by the plugin
// and that the plugin's directory structure is in place.
func ensureFilesystem(path, prefix string, log *logrus.Entry) error {
	info, err := os.Stat(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return errors.Wrap(err, "error checking if bucket/volume exists")
		}
		log.Info("Bucket/Volume does not already exist. Initializing.")
	} else {
		log.Debug("Bucket/Volume already exists")

		if !isWriteable(log, info) {
			log.Debugf("Is path a directory: %+v", info.Mode().IsDir())
			log.Debugf("Directory permissions: %+v", info.Mode().Perm())
			log.Error("Directory is not writable")
			return errors.New("directory is not writeable")
		}

		for _, subdir := range getSubDirectoryLayout() {
			subpath := filepath.Join(path, prefix, subdir)
			if err := os.MkdirAll(subpath, 0755); err != nil {
				return errors.Wrapf(err, "could not create directory %s", subpath)
			}
		}
	}

	return nil
}
