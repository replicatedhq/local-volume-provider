package main

import (
	"github.com/replicatedhq/local-volume-provider/pkg/plugin"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	veleroplugin "github.com/vmware-tanzu/velero/pkg/plugin/framework"
)

func main() {
	veleroplugin.NewServer().
		BindFlags(pflag.CommandLine).
		RegisterObjectStore("replicated.com/hostpath", newHostPathObjectStorePlugin).
		RegisterObjectStore("replicated.com/nfs", newNFSObjectStorePlugin).
		Serve()
}

func newHostPathObjectStorePlugin(logger logrus.FieldLogger) (interface{}, error) {
	return plugin.NewLocalVolumeObjectStore(logger, plugin.Hostpath), nil
}

func newNFSObjectStorePlugin(logger logrus.FieldLogger) (interface{}, error) {
	return plugin.NewLocalVolumeObjectStore(logger, plugin.NFS), nil
}
