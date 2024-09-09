package clientset

import (
	"log"
	"os"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	clientset = &kubernetes.Clientset{}
)

func InitClientset() {
	logrus.Println("initing clientset")
	// restConfig := ctrl.GetConfigOrDie()
	restConfig, err := ctrl.GetConfig()
	if err != nil {
		logrus.Errorf("get kubeconfig err: %v", err)
		os.Exit(1)
	}
	cs, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		log.Printf("new clientset err: %v", err)
		return
	}
	clientset = cs
}

func GetClientset() *kubernetes.Clientset {
	return clientset
}
