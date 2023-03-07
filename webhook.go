package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func mutate(req *admissionv1.AdmissionRequest) admission.Response {
	klog.Infof("Call MutatingWebhookConfiguration")
	pod := corev1.Pod{}
	// Get pod object from request
	if err := json.Unmarshal(req.Object.Raw, &pod); err != nil {
		return admission.Errored(http.StatusForbidden, err)
	}
	klog.Infof("mutate pod %s", pod.Name)
	if pod.Labels == nil {
		pod.Labels = map[string]string{}
	}
	pod.Labels["app"] = pod.Name

	newObj, err := json.Marshal(pod)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(req.Object.Raw, newObj)
}

func validate(req *admissionv1.AdmissionRequest) admission.Response {
	klog.Infof("Call ValidatingWebhookConfiguration")
	pod := corev1.Pod{}

	// Get pod object from request
	if err := json.Unmarshal(req.Object.Raw, &pod); err != nil {
		return admission.Errored(http.StatusForbidden, err)
	}
	klog.Infof("validating pod %s", pod.Name)

	for _, ctr := range pod.Spec.Containers {
		if ctr.Image != "ebpf-test" {
			return admission.Denied(fmt.Sprintf("%s image name not good", ctr.Name))
		}
	}
	return admission.Allowed("")
}
