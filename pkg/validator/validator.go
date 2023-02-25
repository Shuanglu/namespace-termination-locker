package validator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	k8sError "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	klog "k8s.io/klog/v2"
)

type WhiteList struct {
	Group    string `json:"group,omitempty"`
	Version  string `json:"version,omitempty"`
	Resource string `json:"resource,omitempty"`
	Name     string `json:"name,omitempty"`
}

type WhiteLists struct {
	WhiteLists []WhiteList `json:"whitelists"`
}

var (
	whielistPath = "/etc/admission-webhook/whitelist/whitelist.json"
)

func Validate(namespace string) (bool, *meta.Status) {
	klog.Infof("Starting validation")
	config, err := rest.InClusterConfig()
	if err != nil {
		klog.Errorf("Failed to create config: %v")
		return false, &meta.Status{
			Message: fmt.Sprintf("Failed to create config: %v. Please check webhook server.", err),
		}
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Errorf("Failed to create clientset: %v")
		return false, &meta.Status{
			Message: fmt.Sprintf("Failed to create clientset: %v. Please check webhook server.", err),
		}
	}
	klog.V(4).Infof("Clientset has been initialized")
	_, err = clientset.CoreV1().Namespaces().Get(context.TODO(), namespace, meta.GetOptions{})
	if err != nil {
		if k8sError.IsNotFound(err) {
			return false, &meta.Status{
				Message: "The request is to delete a non-existing namespace.",
			}
		}
	}

	// List namespaced apiresources
	klog.V(4).Infof("Starting to list apiresources")
	_, apiresourceLists, err := clientset.DiscoveryClient.ServerGroupsAndResources()
	for _, apiresourceList := range apiresourceLists {
		klog.V(4).Infof("apiresource APIVersion: %v; Kind: %v; Group: %v; API: %v", apiresourceList.APIVersion, apiresourceList.Kind, apiresourceList.GroupVersion, apiresourceList.APIResources)
	}
	//clientset.DiscoveryClient.ServerResourcesForGroupVersion(schema.GroupVersion{})
	if err != nil {
		return false, &meta.Status{
			Message: fmt.Sprintf("Failed to list namespaced apiresources: %v. Please check webhook server", err),
		}
	}
	dclient, err := dynamic.NewForConfig(config)
	if err != nil {
		klog.Errorf("Failed to build dynamic client: %v", err)
	}

	whitelistsBytes, _ := os.ReadFile(whielistPath)
	var whitelists WhiteLists
	err = json.Unmarshal(whitelistsBytes, &whitelists)
	if err != nil {
		klog.Warningf("Failed to parse the whitelist json: %v", err)
	}
	whitelistMemory := make(map[string]WhiteList)
	for _, whitelist := range whitelists.WhiteLists {
		whitelistMemory[whitelist.Resource] = whitelist
		klog.V(3).Infof("Adding %v/%v/%v/%v to the whitelist", whitelist.Group, whitelist.Version, whitelist.Resource, whitelist.Name)
	}
	for _, apiresourceList := range apiresourceLists {
		if len(apiresourceList.APIResources) == 0 {
			continue
		}
		var resource dynamic.ResourceInterface
		var group string
		var version string
		if len(strings.Split(apiresourceList.GroupVersion, "/")) == 2 {
			group = strings.Split(apiresourceList.GroupVersion, "/")[0]
			version = strings.Split(apiresourceList.GroupVersion, "/")[1]
		} else {
			version = strings.Split(apiresourceList.GroupVersion, "/")[0]
			group = ""
		}

		for _, apiresource := range apiresourceList.APIResources {
			klog.V(4).Infof("Checking Group: %v; version: %v; resource: %v, namespace: %v", group, version, apiresource.Kind, namespace)

			groupVersionResource := schema.GroupVersionResource{
				Group:    group,
				Version:  version,
				Resource: apiresource.Name,
			}
			for _, verb := range apiresource.Verbs {
				if strings.ToLower(verb) == "list" && apiresource.Namespaced {
					resource = dclient.Resource(groupVersionResource).Namespace(namespace)
					resourceList, err := resource.List(context.TODO(), meta.ListOptions{})
					if err != nil {
						klog.Errorf("Failed to list \"%v/%v\" resource %q: %v", group, version, apiresource.Kind, err)
					}
					if len(resourceList.Items) >= 1 {
						block := true
						if _, ok := whitelistMemory[apiresource.Name]; ok {
							if whitelistMemory[apiresource.Name].Name == "" {
								block = false
							} else {
								for _, item := range resourceList.Items {
									klog.V(4).Infof("Checking resource %v/%v/%v/%v", group, version, apiresource.Kind, item.GetName())
									if item.GetName() == whitelistMemory[apiresource.Name].Name {
										itemGroupVersionKind := item.GroupVersionKind()
										if itemGroupVersionKind.Group == apiresource.Group && itemGroupVersionKind.Version == apiresource.Version {
											block = false
										} else {
											block = true
										}
									} else {
										block = true
									}
								}
							}
						} else {
							block = true
						}

						if !block {
							continue
						}
						klog.Infof("Deny the request becasue \"%v/%v\" resource %q still exist under the namespace %v", group, version, apiresource.Kind, namespace)
						return false, &meta.Status{
							Message: fmt.Sprintf("Deny the request becasue \"%v/%v\" resource %q still exist under the namespace %v", group, version, apiresource.Kind, namespace),
						}
					}
				}
			}
		}
	}
	klog.Infof("No resources exist in the namespace %v. Delete operation will be allowed", namespace)
	return true, nil
}
