package server

import (
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"encoding/json"

	"github.com/Shuanglu/namespace-termination-locker/pkg/validator"
	admission "k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	klog "k8s.io/klog/v2"
)

func Server() {
	klog.Infof("Server starting")
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

	klog.Infof("Server started")
}

var (
	universalDeserializer = serializer.NewCodecFactory(runtime.NewScheme()).UniversalDeserializer()
	certPath              = "/etc/admission-webhook/tls/tls.crt"
	keyPath               = "/etc/admission-webhook/tls/tls.key"
)

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

	allowed, result := validator.Validate(admissionReviewRequest.Request.Namespace)
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
