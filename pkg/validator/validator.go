package validator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Shuanglu/namespace-termination-locker/pkg/types"
	k8sError "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	klog "k8s.io/klog/v2"
)

var (
	whielistPath     = "/etc/admission-webhook/whitelist/whitelist.json"
	builtinWhitelist = map[string]bool{"event": true}
)

func WhiteListInit(namespace string) map[string]bool {
	whitelistsBytes, err := os.ReadFile(whielistPath)
	if err != nil {
		klog.Errorf("Failed to read the data in %q", whielistPath)
	}
	var whitelists types.WhiteLists
	err = json.Unmarshal(whitelistsBytes, &whitelists)
	if err != nil {
		klog.Warningf("Failed to parse the whitelist json: %v", err)
	}
	whitelistMemory := make(map[string]bool)
	for _, whitelist := range whitelists.WhiteLists {
		if whitelist.Namespace == "" {
			whitelist.Namespace = namespace
		}
		if whitelist.Group == "" {
			whitelistMemory[strings.ToLower(whitelist.Namespace)+"/"+strings.ToLower(whitelist.Version)+"/"+strings.ToLower(whitelist.Kind)+"/"+strings.ToLower(whitelist.Name)] = true
		} else {
			whitelistMemory[strings.ToLower(whitelist.Namespace)+"/"+strings.ToLower(whitelist.Group)+"/"+strings.ToLower(whitelist.Version)+"/"+strings.ToLower(whitelist.Kind)+"/"+strings.ToLower(whitelist.Name)] = true
		}
		klog.V(3).Infof("Adding %v/%v/%v/%v to the whitelist", whitelist.Group, whitelist.Version, whitelist.Kind, whitelist.Name)
	}
	klog.Infof("Loaded the whitelist data")
	return whitelistMemory
}

func Validate(namespace string, kubernetesClient types.KubernetesClient) (bool, *metav1.Status) {
	klog.Infof("Starting validation")
	whitelistMemory := WhiteListInit(namespace)
	_, err := kubernetesClient.Clientset.CoreV1().Namespaces().Get(context.TODO(), namespace, metav1.GetOptions{})
	if err != nil {
		if k8sError.IsNotFound(err) {
			return false, &metav1.Status{
				Message: "The request is to delete a non-existing namespace",
			}
		}
	}
	for _, genericInformer := range kubernetesClient.GenericInformers {
		genericNamespaceLister := genericInformer.Lister().ByNamespace(namespace)
		resources, err := genericNamespaceLister.List(labels.Everything())
		if err != nil {
			return false, &metav1.Status{
				Message: "Failed to get resources under the namespace",
			}
		}
		accessor := meta.NewAccessor()
		for _, resource := range resources {
			deny := false
			name, err := accessor.Name(resource)
			if err != nil {
				klog.Errorf("Failed to get the 'name' from the resource: %v", err)
				deny = true
			}
			kind, err := accessor.Kind(resource)
			if err != nil {
				klog.Errorf("Failed to get the 'kind' from the resource: %v", err)
				deny = true
			}
			apiversion, err := accessor.APIVersion(resource)
			if err != nil {
				klog.Errorf("Failed to get the 'apiversion' from the resource: %v", err)
				deny = true
			}
			if deny {
				return false, &metav1.Status{
					Message: fmt.Sprintf("There is resource existing under the namespace %q but failed to parse the metadta of it: %v", namespace, err),
				}
			}
			klog.V(3).Infof("Checking resource: apiversion %q kind %q namespace %q name %q", strings.ToLower(apiversion), strings.ToLower(kind), strings.ToLower(namespace), strings.ToLower(name))
			if exist := whitelistMemory[strings.ToLower(namespace)+"/"+strings.ToLower(apiversion)+"/"+strings.ToLower(kind)+"/"+strings.ToLower(name)]; exist {
				klog.Infof("The resource exists in the whitelist")
				continue
			} else if builtinWhitelist[strings.ToLower(kind)] {
				klog.Infof("The resource exists in the builtinWhitelist")
				continue
			} else {
				return false, &metav1.Status{
					Message: fmt.Sprintf("The resource %q under the namespace %q still exists and doesn't exist in the whitelist. Please clean up before deleting the namespace.", apiversion+"/"+kind+"/"+name, namespace),
				}
			}
		}
	}

	return true, nil

}
