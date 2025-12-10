package k8s

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// NewClient returns a new Kubernetes clientset.
// It tries in-cluster config first, then falls back to ~/.kube/config.
func NewClient() (*kubernetes.Clientset, error) {
	// 1. Try In-Cluster Config
	config, err := rest.InClusterConfig()
	if err != nil {
		// 2. Fallback to local kubeconfig
		var kubeconfig string
		if home := homedir.HomeDir(); home != "" {
			kubeconfig = filepath.Join(home, ".kube", "config")
		} else {
			kubeconfig = os.Getenv("KUBECONFIG")
		}

		if kubeconfig == "" {
			return nil, fmt.Errorf("could not find kubeconfig and not running in cluster")
		}

		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("failed to build config from %s: %w", kubeconfig, err)
		}
	}

	return kubernetes.NewForConfig(config)
}
