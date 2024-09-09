package handler

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/sirupsen/logrus"
	admission "k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
)

var (
	UniversalDeserializer = serializer.NewCodecFactory(runtime.NewScheme()).UniversalDeserializer()
	replicasetCache       = make(map[types.UID]bool)
)

type mutateHandler struct {
}

func NewMutateHandler() http.Handler {
	return &mutateHandler{}
}

type PatchOperation struct {
	Operation string      `json:"op"`
	Path      string      `json:"path"`
	Value     interface{} `json:"value,omitempty"`
}

func (h *mutateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logrus.Debugln("requested")
	if r.Method != http.MethodPost {
		logrus.Errorln("request method not allowed")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
		logrus.Errorln("content type not application/json")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	requestBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		logrus.Errorf("read request body err: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	logrus.Debugf("request body: %s", requestBody)

	admissionReviewFromRequest, err := extractAdmissionReviewFromRequest(requestBody)
	if err != nil {
		logrus.Errorf("extract admission review from request err: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	raw := admissionReviewFromRequest.Request.Object.Raw
	pod := corev1.Pod{}
	if _, _, err := UniversalDeserializer.Decode(raw, nil, &pod); err != nil {
		logrus.Errorf("decode object to pod err: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if podNotToHandle(pod) {
		logrus.Errorf("pod not to handle: %v", pod.Name)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	nodeAffinity := setNodeAffinity(pod.OwnerReferences[0].UID)
	// nodeAffinity := setNodeAffinity("aaa")

	admissionReviewToResponse, err := buildAdmissionReviewToResponse(admissionReviewFromRequest, nodeAffinity)
	if err != nil {
		logrus.Errorf("build admission review response err: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	admissionReviewResponseBytes, err := json.Marshal(admissionReviewToResponse)
	if err != nil {
		logrus.Errorf("json marshal admission review response err: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	logrus.Debugln("handled successfully")
	w.Write(admissionReviewResponseBytes)
}

func podNotToHandle(pod corev1.Pod) bool {
	if pod.OwnerReferences == nil {
		return true
	}
	if len(pod.OwnerReferences) == 0 {
		return true
	}
	ownerRef := pod.OwnerReferences[0]
	if ownerRef.APIVersion != "apps/v1" || ownerRef.Kind != "ReplicaSet" {
		return true
	}
	return false
}

func setNodeAffinity(ownerRefUID types.UID) corev1.NodeAffinity {
	nodeAffinity := corev1.NodeAffinity{}

	// we just want only one pod with NodeAffinity to on-demand node

	if replicasetCache[ownerRefUID] {
		// the same replicaset already had one pod set NodeAffinity to on-demand node,
		// we want the others pod of the replicaset to get NodeAffinity to spot node
		nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{
				{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{
							Key:      "node.kubernetes.io/capacity",
							Operator: "In",
							Values:   []string{"spot"},
						},
					},
				},
			},
		}
	} else {
		// no pod of the same replicaset has been set NodeAffinity to on-demand node, we set one here
		nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{
				{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{
							Key:      "node.kubernetes.io/capacity",
							Operator: "In",
							Values:   []string{"on-demand"},
						},
					},
				},
			},
		}
		defer func() {
			replicasetCache[ownerRefUID] = true
		}()
	}

	return nodeAffinity
}

func buildAdmissionReviewToResponse(admissionReviewFromRequest admission.AdmissionReview, nodeAffinity corev1.NodeAffinity) (admission.AdmissionReview, error) {
	admissionReviewToResponse := admission.AdmissionReview{
		TypeMeta: admissionReviewFromRequest.TypeMeta,
		Response: &admission.AdmissionResponse{
			UID: admissionReviewFromRequest.Request.UID,
		},
	}
	patchOperations := []PatchOperation{}
	affinity := corev1.Affinity{
		NodeAffinity: &nodeAffinity,
	}
	op := PatchOperation{
		Operation: "add",
		// Path:      "/spec/affinity/nodeAffinity/requiredDuringSchedulingIgnoredDuringExecution",
		Path:  "/spec/affnity",
		Value: affinity,
	}
	patchOperations = append(patchOperations, op)
	patchBytes, err := json.Marshal(patchOperations)
	if err != nil {
		logrus.Errorf("json marshal err: %v", err)
		return admissionReviewToResponse, err
	}

	admissionReviewToResponse.Response.Allowed = true
	admissionReviewToResponse.Response.Patch = patchBytes
	patchTypeJSONPatch := admission.PatchTypeJSONPatch
	admissionReviewToResponse.Response.PatchType = &patchTypeJSONPatch

	return admissionReviewToResponse, nil
}

func extractAdmissionReviewFromRequest(requestBody []byte) (admission.AdmissionReview, error) {
	var admissionReviewFromRequest admission.AdmissionReview
	if _, _, err := UniversalDeserializer.Decode(requestBody, nil, &admissionReviewFromRequest); err != nil {
		logrus.Errorf("deserialize err: %v", err)
		return admissionReviewFromRequest, err
	} else if admissionReviewFromRequest.Request == nil {
		logrus.Errorf("admission review request is nil")
		return admissionReviewFromRequest, err
	}

	return admissionReviewFromRequest, nil
}
