package types

import (
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type KubernetesClient struct {
	Clientset        *kubernetes.Clientset
	Config           *rest.Config
	DynamicClient    *dynamic.DynamicClient
	GenericInformers []informers.GenericInformer
}

type WhiteList struct {
	Group     string `json:"group,omitempty"`
	Version   string `json:"version,omitempty"`
	Kind      string `json:"resource,omitempty"`
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

type WhiteLists struct {
	WhiteLists []WhiteList `json:"whitelists"`
}
