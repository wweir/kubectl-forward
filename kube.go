package main

import (
	"os"
	"path/filepath"

	"github.com/sower-proxy/deferlog/log"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var getClient func() (*rest.Config, *kubernetes.Clientset)

func init() {
	var conf *rest.Config
	var clientset *kubernetes.Clientset

	cfgpath := filepath.Join(os.Getenv("HOME"), ".kube/config")

	if _, err := os.Stat(cfgpath); err == nil {
		conf, err = clientcmd.BuildConfigFromFlags("", cfgpath)
		log.InfoFatal(err).
			Str("path", cfgpath).
			Msg("Failed to load kubernetes config")

	} else {
		conf, err = rest.InClusterConfig()
		log.InfoFatal(err).
			Msg("Failed to load kubernetes in cluster config")
	}

	clientset, err := kubernetes.NewForConfig(conf)
	log.DebugFatal(err).
		Msg("Failed to create kubernetes client")

	getClient = func() (*rest.Config, *kubernetes.Clientset) {
		return conf, clientset
	}
}
