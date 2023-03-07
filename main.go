package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	tlsKeyName  = "tls.key"
	tlsCertName = "tls.crt"
)

func waitExit() {
	exitChan := make(chan os.Signal, 1)
	signal.Notify(exitChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	<-exitChan
	klog.Infof("Got OS shutdown signal, shutting down webhook server gracefully...")
}

var (
	runtimeScheme = runtime.NewScheme()
	codecs        = serializer.NewCodecFactory(runtimeScheme)
	decoder, _    = admission.NewDecoder(runtimeScheme)
	deserializer  = codecs.UniversalDeserializer()
)

func serve(w http.ResponseWriter, r *http.Request) {
	var body []byte
	if r.Body != nil {
		if data, err := io.ReadAll(r.Body); err == nil {
			body = data
		}
	}
	if len(body) == 0 {
		klog.Error("empty body")
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}
	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		klog.Errorf("contentType=%s, expect application/json", contentType)
		return
	}
	var response admission.Response
	ar := admissionv1.AdmissionReview{}
	if _, _, err := deserializer.Decode(body, nil, &ar); err != nil {
		msg := fmt.Sprintf("Request could not be decoded: %v", err)
		klog.Error(msg)
		response = admission.Errored(http.StatusInternalServerError, err)

	} else {
		if r.URL.Path == "/mutate" {
			response = mutate(ar.Request)
		} else if r.URL.Path == "/validate" {
			response = validate(ar.Request)
		}
	}

	if err := response.Complete(admission.Request{AdmissionRequest: *ar.Request}); err != nil {
		klog.Errorf("unable to get response: %v", err)
		http.Error(w, fmt.Sprintf("could not get response: %v", err), http.StatusInternalServerError)
	}

	responseAdmissionReview := admissionv1.AdmissionReview{
		TypeMeta: ar.TypeMeta,
		Response: &response.AdmissionResponse,
	}
	resp, err := json.Marshal(responseAdmissionReview)
	if err != nil {
		klog.Errorf("Can't encode response: %v", err)
		http.Error(w, fmt.Sprintf("could not encode response: %v", err), http.StatusInternalServerError)
	}
	w.Header().Set("Content-Type", "application/json")

	if _, err := w.Write(resp); err != nil {
		klog.Errorf("Can't write response: %v", err)
		http.Error(w, fmt.Sprintf("could not write response: %v", err), http.StatusInternalServerError)
	}

}

func main() {
	port := ":8000"
	if certDir := os.Getenv("CERT_DIR"); certDir != "" {
		certFile := filepath.Join(certDir, tlsCertName)
		keyFile := filepath.Join(certDir, tlsKeyName)
		pair, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			klog.Errorf("Failed to load key pair in $s : %v", certDir, err)
		}

		mux := http.NewServeMux()
		mux.HandleFunc("/validate", serve)
		mux.HandleFunc("/mutate", serve)

		server := &http.Server{
			Addr:      port,
			TLSConfig: &tls.Config{Certificates: []tls.Certificate{pair}},
			Handler:   mux,
		}

		klog.Infof("Server started Listen to %s", port)
		go func() {
			if err := server.ListenAndServeTLS("", ""); err != nil {
				klog.Errorf("Failed to listen and serve webhook server: %v", err)
			}
		}()

		waitExit()
		server.Shutdown(context.Background())

	}

}
