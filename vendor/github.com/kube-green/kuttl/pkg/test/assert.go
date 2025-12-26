package test

import (
	"errors"
	"fmt"
	"time"

	"k8s.io/client-go/discovery"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kube-green/kuttl/pkg/kubernetes"
)

// Assert checks all provided assert files against a namespace.  Upon assert failure, it prints the failures and returns an error
func Assert(namespace string, timeout int, assertFiles ...string) error {
	var objects []client.Object

	for _, file := range assertFiles {
		o, err := ObjectsFromPath(file, "")
		if err != nil {
			return err
		}
		objects = append(objects, o...)
	}

	// feels like the wrong abstraction, need to do some refactoring
	s := &Step{
		Timeout:         0,
		Client:          Client,
		DiscoveryClient: DiscoveryClient,
	}

	var testErrors []error
	for i := 0; i < timeout; i++ {
		// start fresh
		testErrors = []error{}
		for _, expected := range objects {
			testErrors = append(testErrors, s.CheckResource(expected, namespace)...)
		}

		if len(testErrors) == 0 {
			break
		}

		time.Sleep(time.Second)
	}

	if len(testErrors) == 0 {
		fmt.Printf("assert is valid\n")
		return nil
	}

	for _, testError := range testErrors {
		fmt.Println(testError)
	}
	return errors.New("asserts not valid")
}

// Errors checks all provided errors files against a namespace.  Upon assert failure, it prints the failures and returns an error
func Errors(namespace string, timeout int, errorFiles ...string) error {
	var objects []client.Object

	for _, file := range errorFiles {
		o, err := ObjectsFromPath(file, "")
		if err != nil {
			return err
		}
		objects = append(objects, o...)
	}

	// feels like the wrong abstraction, need to do some refactoring
	s := &Step{
		Timeout:         0,
		Client:          Client,
		DiscoveryClient: DiscoveryClient,
	}

	var testErrors []error
	for i := 0; i < timeout; i++ {
		// start fresh
		testErrors = []error{}
		for _, expected := range objects {
			if err := s.CheckResourceAbsent(expected, namespace); err != nil {
				testErrors = append(testErrors, err)
			}
		}

		if len(testErrors) == 0 {
			break
		}

		time.Sleep(time.Second)
	}

	if len(testErrors) == 0 {
		fmt.Printf("error assert is valid\n")
		return nil
	}

	for _, testError := range testErrors {
		fmt.Println(testError)
	}
	return errors.New("error asserts not valid")
}

func Client(_ bool) (client.Client, error) {
	cfg, err := kubernetes.GetConfig()
	if err != nil {
		return nil, err
	}

	client, err := kubernetes.NewRetryClient(cfg, client.Options{
		Scheme: kubernetes.Scheme(),
	})
	if err != nil {
		return nil, fmt.Errorf("fatal error getting client: %v", err)
	}

	return client, nil
}

func DiscoveryClient() (discovery.DiscoveryInterface, error) {
	cfg, err := kubernetes.GetConfig()
	if err != nil {
		return nil, err
	}

	dclient, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("fatal error getting discovery client: %v", err)
	}

	return dclient, nil
}
