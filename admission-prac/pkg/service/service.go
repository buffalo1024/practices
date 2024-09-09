package service

import (
	"context"
	"log"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"practices/admission-prac/pkg/clientset"
)

type ServiceParameters struct {
	Name      string
	Namespace string
	Selector  map[string]string
	Ports     []corev1.ServicePort
}

func CreateService(parameters ServiceParameters) {
	svc := corev1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:      parameters.Name,
			Namespace: parameters.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: parameters.Selector,
			Ports:    parameters.Ports,
		},
	}

	cs := clientset.GetClientset()
	log.Printf("to create svc")
	if _, err := cs.CoreV1().Services("").Create(context.TODO(), &svc, v1.CreateOptions{}); err != nil {
		log.Printf("create svc err: %v", err)
		return
	}
}
