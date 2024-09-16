package handler

import (
	"practices/admission-prac/pkg/clientset"
	"time"

	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

var (
	replicasetCache              = make(map[types.UID]PodCachemap)
	replicasetsOfNoneDeployments = make(map[types.UID]bool)
)

type PodCachemap map[types.UID]corev1.Pod

type replicasetEventHandler struct {
}

func (h *replicasetEventHandler) OnAdd(obj interface{}) {
	replicaset := obj.(*appsv1.ReplicaSet)
	ownerReferences := replicaset.GetOwnerReferences()
	if ownerReferences == nil {
		replicasetsOfNoneDeployments[replicaset.UID] = true
		return
	}
	if len(ownerReferences) == 0 {
		replicasetsOfNoneDeployments[replicaset.UID] = true
		return
	}
	ownerRef := ownerReferences[0]
	if ownerRef.APIVersion != "apps/v1" || ownerRef.Kind != "Deployment" {
		replicasetsOfNoneDeployments[replicaset.UID] = true
		return
	}
}

func (h *replicasetEventHandler) OnUpdate(oldObj, newObj interface{}) {
	h.OnAdd(newObj)
}

func (h *replicasetEventHandler) OnDelete(obj interface{}) {
	replicaset := obj.(*appsv1.ReplicaSet)
	delete(replicasetsOfNoneDeployments, replicaset.UID)
}

type podEventHandler struct {
}

func (h *podEventHandler) OnAdd(obj interface{}) {
	pod := obj.(*corev1.Pod)
	ownerReferences := pod.GetOwnerReferences()
	if ownerReferences == nil {
		return
	}
	if len(ownerReferences) == 0 {
		return
	}
	ownerRef := pod.OwnerReferences[0]
	if ownerRef.APIVersion != "apps/v1" || ownerRef.Kind != "ReplicaSet" {
		return
	}
	podCacheMap, ok := replicasetCache[ownerRef.UID]
	if !ok {
		podCacheMap = make(PodCachemap)
	}
	podCacheMap[pod.UID] = *pod
	replicasetCache[ownerRef.UID] = podCacheMap
}

func (h *podEventHandler) OnUpdate(oldObj, newObj interface{}) {
	pod := newObj.(*corev1.Pod)
	ownerReferences := pod.GetOwnerReferences()
	if ownerReferences == nil {
		return
	}
	if len(ownerReferences) == 0 {
		return
	}
	ownerRef := pod.OwnerReferences[0]
	if ownerRef.APIVersion != "apps/v1" || ownerRef.Kind != "ReplicaSet" {
		return
	}
	podCacheMap, ok := replicasetCache[ownerRef.UID]
	if !ok {
		podCacheMap = make(PodCachemap)
	}
	podCacheMap[pod.UID] = *pod
	replicasetCache[ownerRef.UID] = podCacheMap
}

func (h *podEventHandler) OnDelete(obj interface{}) {
	pod := obj.(*corev1.Pod)
	ownerReferences := pod.GetOwnerReferences()
	if ownerReferences == nil {
		return
	}
	if len(ownerReferences) == 0 {
		return
	}
	ownerRef := pod.OwnerReferences[0]
	if ownerRef.APIVersion != "apps/v1" || ownerRef.Kind != "ReplicaSet" {
		return
	}
	podCacheMap, ok := replicasetCache[ownerRef.UID]
	if !ok {
		podCacheMap = make(PodCachemap)
		return
	}
	delete(podCacheMap, pod.UID)
	replicasetCache[ownerRef.UID] = podCacheMap
}

func StartInformer(stopCh <-chan struct{}) {
	logrus.Debug("staring informer")
	cs := clientset.GetClientset()
	informerFactory := informers.NewSharedInformerFactory(cs, time.Duration(time.Second))

	podInformer := informerFactory.Core().V1().Pods().Informer()
	ph := &podEventHandler{}
	podInformer.AddEventHandler(ph)
	rsInformer := informerFactory.Apps().V1().ReplicaSets().Informer()
	rsh := &replicasetEventHandler{}
	rsInformer.AddEventHandler(rsh)

	logrus.Debug("to start informer")
	informerFactory.Start(stopCh)

	logrus.Debug("to sync cache")
	if !cache.WaitForCacheSync(stopCh, podInformer.HasSynced, rsInformer.HasSynced) {
		logrus.Error("failed to sync cache")
		return
	}
	logrus.Debug("cache synced")
}
