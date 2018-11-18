/*
Copyright 2018 Microsoft - Idan Levin and Dimitris-Ilias Gkanatsios
Copyright 2018 The Kubernetes Authors.

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

package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/golang/glog"
	"k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

// Config contains the server (the webhook) cert and key.
type Config struct {
	CertFile string
	KeyFile  string
}

func (c *Config) addFlags() {
	flag.StringVar(&c.CertFile, "tls-cert-file", c.CertFile, ""+
		"File containing the default x509 Certificate for HTTPS. (CA cert, if any, concatenated "+
		"after server cert).")
	flag.StringVar(&c.KeyFile, "tls-private-key-file", c.KeyFile, ""+
		"File containing the default x509 private key matching --tls-cert-file.")
}

func configTLS(config Config) *tls.Config {
	sCert, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)

	if err != nil {
		glog.Fatal(err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{sCert},
	}
}

var (
	runtimeScheme = runtime.NewScheme()
	codecs        = serializer.NewCodecFactory(runtimeScheme)
	deserializer  = codecs.UniversalDeserializer()
)

// toAdmissionResponse is a helper function to create an AdmissionResponse
// with an embedded error
func toAdmissionResponse(err error) *v1beta1.AdmissionResponse {
	return &v1beta1.AdmissionResponse{
		Result: &metav1.Status{
			Message: err.Error(),
		},
	}
}

// admitFunc is the type we use for all of our validators and mutators
type admitFunc func(v1beta1.AdmissionReview) *v1beta1.AdmissionResponse

// serve handles the http portion of a request prior to handing to an admit func
func serve(w http.ResponseWriter, r *http.Request, admit admitFunc) {
	var body []byte
	if r.Body != nil {
		if data, err := ioutil.ReadAll(r.Body); err == nil {
			body = data
		}
	}

	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		glog.Errorf("contentType=%s, expect application/json", contentType)
		return
	}

	glog.V(2).Info(fmt.Sprintf("handling request: %s", body))

	// the AdmissionReview that was sent to the webhook
	requestedAdmissionReview := v1beta1.AdmissionReview{}

	// the AdmissionReview that will be returned
	responseAdmissionReview := v1beta1.AdmissionReview{}

	deserializer := codecs.UniversalDeserializer()
	if _, _, err := deserializer.Decode(body, nil, &requestedAdmissionReview); err != nil {
		glog.Error(err)
		responseAdmissionReview.Response = toAdmissionResponse(err)
	} else {
		// pass to admitFunc
		responseAdmissionReview.Response = admit(requestedAdmissionReview)
	}

	// return the same UID
	responseAdmissionReview.Response.UID = requestedAdmissionReview.Request.UID

	glog.V(2).Info(fmt.Sprintf("sending response: %v", responseAdmissionReview.Response))

	respBytes, err := json.Marshal(responseAdmissionReview)
	if err != nil {
		glog.Error(err)
	}
	if _, err := w.Write(respBytes); err != nil {
		glog.Error(err)
	}
}

// only allow pods that specify resource request and limit
func admitPods(ar v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
	glog.V(2).Info("admitting pods")

	// make sure the resource is a Pod
	podResource := metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	if ar.Request.Resource != podResource {
		err := fmt.Errorf("expect resource to be %s", podResource)
		glog.Error(err)
		return toAdmissionResponse(err)
	}

	// deserialize Pod spec
	raw := ar.Request.Object.Raw
	pod := corev1.Pod{}
	if _, _, err := deserializer.Decode(raw, nil, &pod); err != nil {
		glog.Error(err)
		return toAdmissionResponse(err)
	}

	response := v1beta1.AdmissionResponse{}
	response.Allowed = true

	var msg string
	// iterate over the pod containers
	for _, container := range pod.Spec.Containers {
		glog.V(2).Info(fmt.Sprintf("Validating container name: %s", container.Name))
		requests := container.Resources.Requests
		// check for requests
		if requests.Cpu() == nil || requests.Cpu().IsZero() ||
			requests.Memory() == nil || requests.Memory().IsZero() {
			response.Allowed = false
			msg = fmt.Sprintf("Container '%s' does not have CPU and/or Memory requests defined", container.Name)
			break
		}

		limits := container.Resources.Limits
		// check for limits
		if limits.Cpu() == nil || limits.Cpu().IsZero() ||
			limits.Memory() == nil || limits.Memory().IsZero() {
			response.Allowed = false
			msg = fmt.Sprintf("Container '%s' does not have CPU and/or Memory limits defined", container.Name)
			break
		}
	}

	if !response.Allowed {
		response.Result = &metav1.Status{Message: strings.TrimSpace(msg)}
	}

	return &response
}

func servePods(w http.ResponseWriter, r *http.Request) {
	serve(w, r, admitPods)
}

func main() {
	var config Config
	config.addFlags()
	flag.Parse()

	http.HandleFunc("/pods", servePods)

	glog.V(2).Info("Configuring TLS")

	server := &http.Server{
		Addr:      ":8443",
		TLSConfig: configTLS(config),
	}

	glog.V(2).Info("Listening for admission validation requests")

	server.ListenAndServeTLS("", "")
}
