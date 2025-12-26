package kubernetes

import (
	"io"

	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	appsv1 "k8s.io/client-go/tools/clientcmd/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

var ImpersonateAs = ""

func GetConfig() (*rest.Config, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	if ImpersonateAs != "" {
		cfg.Impersonate.UserName = ImpersonateAs
	}

	return cfg, nil
}

func BuildConfigWithContext(kubeconfigPath, context string) (*rest.Config, error) {
	if context == "" {
		// Use default context
		return clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	}

	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath},
		&clientcmd.ConfigOverrides{CurrentContext: context}).ClientConfig()
}

// Kubeconfig converts a rest.Config into a YAML kubeconfig and writes it to w
func Kubeconfig(cfg *rest.Config, w io.Writer) error {
	var authProvider *appsv1.AuthProviderConfig
	var execConfig *appsv1.ExecConfig
	if cfg.AuthProvider != nil {
		authProvider = &appsv1.AuthProviderConfig{
			Name:   cfg.AuthProvider.Name,
			Config: cfg.AuthProvider.Config,
		}
	}

	if cfg.ExecProvider != nil {
		execConfig = &appsv1.ExecConfig{
			Command:    cfg.ExecProvider.Command,
			Args:       cfg.ExecProvider.Args,
			APIVersion: cfg.ExecProvider.APIVersion,
			Env:        []appsv1.ExecEnvVar{},
		}

		for _, envVar := range cfg.ExecProvider.Env {
			execConfig.Env = append(execConfig.Env, appsv1.ExecEnvVar{
				Name:  envVar.Name,
				Value: envVar.Value,
			})
		}
	}
	err := rest.LoadTLSFiles(cfg)
	if err != nil {
		return err
	}
	return json.NewYAMLSerializer(json.DefaultMetaFactory, nil, nil).Encode(&appsv1.Config{
		CurrentContext: "cluster",
		Clusters: []appsv1.NamedCluster{
			{
				Name: "cluster",
				Cluster: appsv1.Cluster{
					Server:                   cfg.Host,
					CertificateAuthorityData: cfg.TLSClientConfig.CAData,
					InsecureSkipTLSVerify:    cfg.TLSClientConfig.Insecure,
				},
			},
		},
		Contexts: []appsv1.NamedContext{
			{
				Name: "cluster",
				Context: appsv1.Context{
					Cluster:  "cluster",
					AuthInfo: "user",
				},
			},
		},
		AuthInfos: []appsv1.NamedAuthInfo{
			{
				Name: "user",
				AuthInfo: appsv1.AuthInfo{
					ClientCertificateData: cfg.TLSClientConfig.CertData,
					ClientKeyData:         cfg.TLSClientConfig.KeyData,
					Token:                 cfg.BearerToken,
					Username:              cfg.Username,
					Password:              cfg.Password,
					Impersonate:           cfg.Impersonate.UserName,
					ImpersonateGroups:     cfg.Impersonate.Groups,
					ImpersonateUserExtra:  cfg.Impersonate.Extra,
					AuthProvider:          authProvider,
					Exec:                  execConfig,
				},
			},
		},
	}, w)
}
