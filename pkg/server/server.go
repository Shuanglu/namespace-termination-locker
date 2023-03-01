package server

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"encoding/json"

	"github.com/Shuanglu/namespace-termination-locker/pkg/types"
	"github.com/Shuanglu/namespace-termination-locker/pkg/validator"
	admission "k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	klog "k8s.io/klog/v2"
)

var (
	universalDeserializer = serializer.NewCodecFactory(runtime.NewScheme()).UniversalDeserializer()
	certPath              = "/etc/admission-webhook/tls/tls.crt"
	keyPath               = "/etc/admission-webhook/tls/tls.key"
	err                   error
	kubernetesClient      types.KubernetesClient
	stopper               = make(chan struct{})
)

func Server() {
	klog.Infof("Server starting")
	if kubernetesClient, err = clientInit(); err != nil {
		klog.Errorf("Failed to initialize the k8s client: %v", err)
	}
	klog.Infof("client init completed")
	var TLS bool
	if _, err := os.Stat(certPath); err == nil {
		if _, err = os.Stat(keyPath); err == nil {
			TLS = true
		}
	}
	http.Handle("/validate", validateHandler{})

	if TLS {
		klog.V(2).Infof("TLS pair exist")
		s := &http.Server{
			Addr:         ":443",
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		}
		if err := s.ListenAndServeTLS(certPath, keyPath); err != nil {
			klog.Errorf("Failed to start the webhook server :%v", err)
		}
	} else {
		klog.V(2).Infof("plain http server")
		s := &http.Server{
			Addr:         ":8080",
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		}
		if err := s.ListenAndServe(); err != nil {
			klog.Errorf("Failed to start the webhook server: %v", err)
		}
	}
}

type validateHandler struct{}

func (validateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		klog.Errorf("This is not a POST request. Review procedure aborting")
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		klog.Errorf("Failed to load request body: %v", err)
		return
	}

	if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
		w.WriteHeader(http.StatusBadRequest)
		klog.Errorf("%v is not supported. only 'application/json' is supported", contentType)
		return
	}

	var admissionReviewRequest admission.AdmissionReview

	if _, _, err = universalDeserializer.Decode(body, nil, &admissionReviewRequest); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		klog.Errorf("Failed to deserialize request: %v", err)
		return
	} else if admissionReviewRequest.Request == nil {
		w.WriteHeader(http.StatusBadRequest)
		klog.Errorf("review request is nil")
		return
	}

	allowed, result := validator.Validate(admissionReviewRequest.Request.Namespace, kubernetesClient)
	klog.V(2).Infof("Allowed: %t", allowed)
	var admissionReviewResponse admission.AdmissionReview
	if allowed {
		admissionReviewResponse = admission.AdmissionReview{
			TypeMeta: admissionReviewRequest.TypeMeta,
			Response: &admission.AdmissionResponse{
				UID:     admissionReviewRequest.Request.UID,
				Allowed: allowed,
			},
		}
	} else {
		admissionReviewResponse = admission.AdmissionReview{
			TypeMeta: admissionReviewRequest.TypeMeta,
			Response: &admission.AdmissionResponse{
				UID:     admissionReviewRequest.Request.UID,
				Allowed: allowed,
				Result:  result,
			},
		}
	}
	bytes, err := json.Marshal(&admissionReviewResponse)
	if err != nil {
		klog.Errorf("Failed to encode response: %v")
		return
	}
	_, err = w.Write(bytes)
	if err != nil {
		klog.Errorf("Failed to write response: %v", err)
		return
	}
	klog.V(2).Infof("Response sent")
}

func clientInit() (types.KubernetesClient, error) {
	kubernetesClient := types.KubernetesClient{}

	defer close(stopper)

	kubernetesClient.Config, err = rest.InClusterConfig()
	if err != nil {
		return kubernetesClient, fmt.Errorf("failed to build config object with the service account: %v", err)
	}
	kubernetesClient.Clientset, err = kubernetes.NewForConfig(kubernetesClient.Config)
	if err != nil {
		return kubernetesClient, fmt.Errorf("failed to create clientset: %v", err)
	}
	klog.V(4).Infof("Clientset has been initialized")

	kubernetesClient.DynamicClient, err = dynamic.NewForConfig(kubernetesClient.Config)
	if err != nil {
		return kubernetesClient, fmt.Errorf("failed to build dynamic client: %v", err)
	}
	sharedInformerFactory := dynamicinformer.NewDynamicSharedInformerFactory(kubernetesClient.DynamicClient, 0)

	// List namespaced apiresources
	klog.V(4).Infof("Starting to list apiresources")
	_, apiresourceLists, err := kubernetesClient.Clientset.DiscoveryClient.ServerGroupsAndResources()
	if err != nil {
		return kubernetesClient, fmt.Errorf("failed to list namespaced apiresources: %v", err)
	}
	for _, apiresourceList := range apiresourceLists {
		if len(apiresourceList.APIResources) == 0 {
			continue
		}
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
			klog.V(4).Infof("Checking apigroup: %v; version: %v; resource: %v", group, version, apiresource.Kind)
			validAPIResource := false
			for _, verb := range apiresource.Verbs {
				if apiresource.Namespaced && strings.ToLower(verb) == "list" {
					validAPIResource = true
				}
			}
			if !validAPIResource {
				continue
			}
			groupVersionResource := schema.GroupVersionResource{
				Group:    group,
				Version:  version,
				Resource: apiresource.Name,
			}
			genericInformer := sharedInformerFactory.ForResource(groupVersionResource)
			go genericInformer.Informer().Run(stopper)
			if !cache.WaitForCacheSync(stopper, genericInformer.Informer().HasSynced) {
				klog.Errorf("failed to sync cache: %v", err)
			}
			kubernetesClient.GenericInformers = append(kubernetesClient.GenericInformers, genericInformer)

		}
	}

	return kubernetesClient, nil
}
