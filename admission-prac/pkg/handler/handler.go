package handler

import (
	"encoding/json"
	"io/ioutil"
	"math/rand"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
	admission "k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
)

type NodeKind string

const (
	NodeOnDemand NodeKind = "on-demand"
	NodeSpot     NodeKind = "spot"
)

var (
	UniversalDeserializer      = serializer.NewCodecFactory(runtime.NewScheme()).UniversalDeserializer()
	nodeSelectorRequirementKey = "node.kubernetes.io/capacity"
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
	requestMark := rand.Int()
	startTime := time.Now()
	logrus.Debugf("requested, requestMark: %v, startTime: %v", requestMark, startTime)
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

	logrus.Debugf("admission review patch to response: %s", admissionReviewToResponse.Response.Patch)
	admissionReviewResponseBytes, err := json.Marshal(admissionReviewToResponse)
	if err != nil {
		logrus.Errorf("json marshal admission review response err: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	logrus.Debugln("handled successfully")
	_, writeErr := w.Write(admissionReviewResponseBytes)
	if writeErr != nil {
		logrus.WithError(writeErr).Error("write response err")
		return
	}
	logrus.Debug("write response successfully")
	endTime := time.Now()
	elapesdTime := endTime.Sub(startTime)
	logrus.Debugf("request ended, requestMark: %v, endTime: %v, elapesdTime: %v", requestMark, endTime, elapesdTime)
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
	if replicasetsOfNoneDeployments[ownerRef.UID] {
		return true
	}
	return false
}

func setNodeAffinity(ownerRefUID types.UID) corev1.NodeAffinity {
	nodeAffinity := corev1.NodeAffinity{}

	podCachemap, ok := replicasetCache[ownerRefUID]
	if !ok {
		// we can know no pods of the replicaset has came out, of courese including one with on-demand node affinity
		podCachemap = make(PodCachemap)
		replicasetCache[ownerRefUID] = podCachemap

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

		return nodeAffinity
	}

	otherPodHasOnDemandNodeAffinity := false
	for _, pod := range podCachemap {
		if podHasOnDemandNodeAffinity(pod) {
			otherPodHasOnDemandNodeAffinity = true
			break
		}
	}

	// we just want only one pod with NodeAffinity to on-demand node

	if otherPodHasOnDemandNodeAffinity {
		// if replicasetCache[ownerRefUID] {
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
		Path:  "/spec/affinity",
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

func podHasOnDemandNodeAffinity(pod corev1.Pod) bool {
	if pod.Spec.Affinity == nil {
		return false
	}
	if pod.Spec.Affinity.NodeAffinity == nil {
		return false
	}
	if pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		return false
	}
	if pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms == nil {
		return false
	}
	for _, term := range pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {
		for _, expression := range term.MatchExpressions {
			if expression.Key == nodeSelectorRequirementKey {
				if expression.Operator == corev1.NodeSelectorOpIn {
					for _, value := range expression.Values {
						if value == string(NodeOnDemand) {
							return true
						}
					}
				}
			}
		}
	}
	return false
}
