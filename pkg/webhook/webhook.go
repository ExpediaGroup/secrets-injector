/*
Copyright (C) 2019 Expedia Group.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package webhook

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/golang/glog"
	"k8s.io/api/admission/v1beta1"
	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/kubernetes/pkg/apis/core/v1"
)

const (
	secretFormatLabel   = "expediagroup.com/secrets-injector-format"
	secretKeyLabel      = "expediagroup.com/secrets-injector-key"
	secretFormatDefault = "toml"
)

var (
	runtimeScheme = runtime.NewScheme()
	codecs        = serializer.NewCodecFactory(runtimeScheme)
	deserializer  = codecs.UniversalDeserializer()

	// (https://github.com/kubernetes/kubernetes/issues/57982)
	defaulter = runtime.ObjectDefaulter(runtimeScheme)
)

type WebhookServer struct {
	Server     *http.Server
	Parameters WhSvrParameters
}

// Webhook Server parameters
type WhSvrParameters struct {
	Port         int    // webhook server port
	CertFile     string // path to the x509 certificate for https
	KeyFile      string // path to the x509 private key matching `CertFile`
	Image        string // Secret manager image
	SecretVolume string // Volume to place the secret
	Command      string // Secret Image Executable override
	CommandArg   string // Secret Image Command Arg override
}

type patchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

func init() {
	_ = corev1.AddToScheme(runtimeScheme)
	_ = admissionregistrationv1beta1.AddToScheme(runtimeScheme)
	// defaulting with webhooks:
	// https://github.com/kubernetes/kubernetes/issues/57982
	_ = v1.AddToScheme(runtimeScheme)
}

// Create volume to mount the secret on
func addSecretsVolume(pod corev1.Pod) (patch []patchOperation) {

	volume := corev1.Volume{
		Name: "secrets",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{Medium: corev1.StorageMediumMemory},
		},
	}

	path := "/spec/volumes"
	var value interface{}

	if len(pod.Spec.Volumes) != 0 {
		path = path + "/-"
		value = volume
	} else {
		value = []corev1.Volume{volume}
	}

	patch = append(patch, patchOperation{
		Op:    "add",
		Path:  path,
		Value: value,
	})

	return
}

// Mount the volume in the container
func addVolumeMount(pod corev1.Pod) (patch []patchOperation) {

	containers := pod.Spec.Containers

	volumeMount := corev1.VolumeMount{
		Name:      "secrets",
		MountPath: "/secrets",
	}

	modifiedContainers := []corev1.Container{}

	for _, container := range containers {
		container.VolumeMounts = appendVolumeMountIfMissing(container.VolumeMounts, volumeMount)
		modifiedContainers = append(modifiedContainers, container)
	}

	patch = append(patch, patchOperation{
		Op:    "replace",
		Path:  "/spec/containers",
		Value: modifiedContainers,
	})

	return
}

// Add volume to existing list of volume mounts in the main container
func appendVolumeMountIfMissing(slice []corev1.VolumeMount, v corev1.VolumeMount) []corev1.VolumeMount {
	for _, ele := range slice {
		if ele == v {
			return slice
		}
	}
	return append(slice, v)
}

// Create init container with secret
func initContainers(pod corev1.Pod, secretKey, secretFormat string, parameters WhSvrParameters) (patch []patchOperation) {
	initContainers := []corev1.Container{}

	secretInjectorContainer := corev1.Container{
		Image:   parameters.Image,
		Name:    "secrets-injector",
		Command: []string{parameters.Command},
		Args:    []string{parameters.CommandArg, secretKey, parameters.SecretVolume, secretFormat},
		VolumeMounts: []corev1.VolumeMount{
			corev1.VolumeMount{
				Name:      "secrets",
				MountPath: parameters.SecretVolume,
			},
		},
	}

	initContainers = append(initContainers, secretInjectorContainer)

	var initOp string
	if len(pod.Spec.InitContainers) != 0 {
		initContainers = append(initContainers, pod.Spec.InitContainers...)
		initOp = "replace"
	} else {
		initOp = "add"
	}

	glog.V(4).Infof("Patch operation %s", initOp)

	patch = append(patch, patchOperation{
		Op:    initOp,
		Path:  "/spec/initContainers",
		Value: initContainers,
	})

	return
}

func createPatch(pod corev1.Pod, secretKey string, secretFormat string, parameters WhSvrParameters) ([]byte, error) {
	var patch []patchOperation

	patch = append(patch, addSecretsVolume(pod)...)
	patch = append(patch, initContainers(pod, secretKey, secretFormat, parameters)...)
	patch = append(patch, addVolumeMount(pod)...)

	return json.Marshal(patch)
}

// main mutation process
func (whsvr *WebhookServer) mutate(ar *v1beta1.AdmissionReview, parameters WhSvrParameters) *v1beta1.AdmissionResponse {
	req := ar.Request

	glog.V(3).Infof("AdmissionReview for Kind=%v, Namespace=%v, UID=%v patchOperation=%v, UserInfo=%v",
		req.Kind, req.Namespace, req.UID, req.Operation, req.UserInfo)

	var pod corev1.Pod
	switch req.Kind.Kind {
	case "Pod":
		if err := json.Unmarshal(req.Object.Raw, &pod); err != nil {
			glog.Errorf("Could not unmarshal raw object: %v", err)
			return &v1beta1.AdmissionResponse{
				Result: &metav1.Status{
					Message: err.Error(),
				},
			}
		}
		glog.V(3).Infof("Discovered Pod Definition: %+v", pod)

		secretFormat := pod.GetLabels()[secretFormatLabel]
		secretKey := pod.GetLabels()[secretKeyLabel]

		if secretKey == "" {
			glog.V(2).Info("No secret key annotation not patching")
		} else {
			if secretFormat == "" {
				secretFormat = secretFormatDefault
			}

			glog.V(4).Infof("Secret key found, creating patch for pod %s", pod.Name)

			patchBytes, err := createPatch(pod, secretKey, secretFormat, parameters)
			if err != nil {
				return &v1beta1.AdmissionResponse{
					Result: &metav1.Status{
						Message: err.Error(),
					},
				}
			}

			glog.V(3).Infof("AdmissionResponse: patch=%v\n", string(patchBytes))
			return &v1beta1.AdmissionResponse{
				Allowed: true,
				Patch:   patchBytes,
				PatchType: func() *v1beta1.PatchType {
					pt := v1beta1.PatchTypeJSONPatch
					return &pt
				}(),
			}
		}
	}
	return &v1beta1.AdmissionResponse{
		Allowed: true,
	}
}

// Serve method for webhook server
func (whsvr *WebhookServer) Serve(w http.ResponseWriter, r *http.Request) {
	var body []byte
	if r.Body != nil {
		if data, err := ioutil.ReadAll(r.Body); err == nil {
			body = data
		}
	}
	if len(body) == 0 {
		glog.Error("empty body in http request")
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}

	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		glog.Errorf("Content-Type=%s, expect application/json", contentType)
		http.Error(w, "invalid Content-Type, expect `application/json`", http.StatusUnsupportedMediaType)
		return
	}

	var admissionResponse *v1beta1.AdmissionResponse
	ar := v1beta1.AdmissionReview{}
	if _, _, err := deserializer.Decode(body, nil, &ar); err != nil {
		glog.Errorf("Can't decode body: %v", err)
		admissionResponse = &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	} else {
		if r.URL.Path == "/mutate" {
			admissionResponse = whsvr.mutate(&ar, whsvr.Parameters)
		}
	}

	admissionReview := v1beta1.AdmissionReview{}
	if admissionResponse != nil {
		admissionReview.Response = admissionResponse
		if ar.Request != nil {
			admissionReview.Response.UID = ar.Request.UID
		}
	}

	resp, err := json.Marshal(admissionReview)
	if err != nil {
		glog.Errorf("Can't encode response: %v", err)
		http.Error(w, fmt.Sprintf("could not encode response: %v", err), http.StatusInternalServerError)
		return
	}
	glog.V(3).Infof("Ready to write response ...")
	if _, err := w.Write(resp); err != nil {
		glog.Errorf("Can't write response: %v", err)
		http.Error(w, fmt.Sprintf("could not write response: %v", err), http.StatusInternalServerError)
		return
	}
}
