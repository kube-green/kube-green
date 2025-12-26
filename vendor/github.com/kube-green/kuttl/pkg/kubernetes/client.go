package kubernetes

import (
	"k8s.io/client-go/discovery"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewClientFunc returns a function which creates a controller-runtime client.
func NewClientFunc(kubeconfigPath, context string) func(bool) (client.Client, error) {
	return func(bool) (client.Client, error) {
		config, err := BuildConfigWithContext(kubeconfigPath, context)
		if err != nil {
			return nil, err
		}

		return NewRetryClient(config, client.Options{
			Scheme: Scheme(),
		})
	}
}

// NewDiscoveryClientFunc returns a function which creates a discovery client.
func NewDiscoveryClientFunc(kubeconfigPath, context string) func() (discovery.DiscoveryInterface, error) {
	return func() (discovery.DiscoveryInterface, error) {
		config, err := BuildConfigWithContext(kubeconfigPath, context)
		if err != nil {
			return nil, err
		}

		return discovery.NewDiscoveryClientForConfig(config)
	}
}
