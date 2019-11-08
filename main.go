package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"

	"k8s.io/api/admission/v1beta1"
	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/klog"
)

var (
	args          Args
	runtimeScheme = runtime.NewScheme()
	codecs        = serializer.NewCodecFactory(runtimeScheme)
	deserializer  = codecs.UniversalDeserializer()
)

type Args struct {
	port     int
	certFile string
	keyFile  string
}

func init() {
	klog.InitFlags(nil)

	flag.IntVar(&args.port, "port", 8080, "Webhook server port.")
	flag.StringVar(&args.certFile, "tlsCertFile", "/etc/webhook/certs/cert.pem", "File containing the x509 Certificate for HTTPS.")
	flag.StringVar(&args.keyFile, "tlsKeyFile", "/etc/webhook/certs/key.pem", "File containing the x509 private key to --tlsCertFile.")
	flag.Parse()

	_ = corev1.AddToScheme(runtimeScheme)
	_ = admissionregistrationv1beta1.AddToScheme(runtimeScheme)
	_ = corev1.AddToScheme(runtimeScheme)
}

func main() {
	klog.Infof("start denyEmptyNodeSelector Server, Listen :%v", args.port)
	http.HandleFunc("/validate", validateHandler)
	err := http.ListenAndServeTLS(fmt.Sprintf(":%v", args.port), args.certFile, args.keyFile, nil)
	if err != nil {
		klog.Fatal("ListenAndServeTLS Error, err=", err)
	}
}

func validateHandler(w http.ResponseWriter, r *http.Request) {
	klog.Info("start validate")
	var body []byte
	if r.Body != nil {
		if data, err := ioutil.ReadAll(r.Body); err == nil {
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
		klog.Errorf("Content-Type=%s, expect application/json", contentType)
		http.Error(w, "invalid Content-Type, expect `application/json`", http.StatusUnsupportedMediaType)
		return
	}
	var admissionResponse = &v1beta1.AdmissionResponse{
		Allowed: true,
	}
	ar := v1beta1.AdmissionReview{}
	if _, _, err := deserializer.Decode(body, nil, &ar); err != nil {
		klog.Errorf("Can't decode body: %v", err)
		admissionResponse = &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	req := ar.Request
	klog.Infof("AdmissionReview for Kind=%v, Namespace=%v Name=%v ,UID=%v patchOperation=%v UserInfo=%v",
		req.Kind, req.Namespace, req.Name, req.UID, req.Operation, req.UserInfo)

	//验证是否设置了nodeSelector
	switch req.Kind.Kind {
	case "Deployment":
		var deployment appsv1.Deployment
		if err := json.Unmarshal(req.Object.Raw, &deployment); err != nil {
			klog.Errorf("Could not unmarshal raw object: %v", err)
			admissionResponse = &v1beta1.AdmissionResponse{
				Result: &metav1.Status{
					Message: err.Error(),
				},
			}
		} else {
			if len(deployment.Spec.Template.Spec.NodeSelector) == 0 {
				klog.Infof("拒绝添加 POD，%s", deployment.Name)
				admissionResponse = &v1beta1.AdmissionResponse{
					Allowed: false,
					Result: &metav1.Status{
						Reason: "required nodeSelector are not set",
					},
				}
			}
		}
	case "Pod":
		var pod corev1.Pod
		if err := json.Unmarshal(req.Object.Raw, &pod); err != nil {
			klog.Errorf("Could not unmarshal raw object: %v", err)
			admissionResponse = &v1beta1.AdmissionResponse{
				Result: &metav1.Status{
					Message: err.Error(),
				},
			}
		} else {
			if len(pod.Spec.NodeSelector) == 0 {
				klog.Infof("拒绝添加 POD，%s", pod.Name)
				admissionResponse = &v1beta1.AdmissionResponse{
					Allowed: false,
					Result: &metav1.Status{
						Reason: "required nodeSelector are not set",
					},
				}
			}
		}
	}

	admissionReview := v1beta1.AdmissionReview{}
	admissionReview.Response = admissionResponse
	if ar.Request != nil {
		admissionReview.Response.UID = ar.Request.UID
	}

	resp, err := json.Marshal(admissionReview)
	if err != nil {
		klog.Errorf("Can't encode response: %v", err)
		http.Error(w, fmt.Sprintf("could not encode response: %v", err), http.StatusInternalServerError)
	}
	klog.Infof("Ready to write reponse ...")
	if _, err := w.Write(resp); err != nil {
		klog.Errorf("Can't write response: %v", err)
		http.Error(w, fmt.Sprintf("could not write response: %v", err), http.StatusInternalServerError)
	}

}
