package main

import (
	"flag"
	"fmt"
	"net/http"
	"path"
	"runtime"
	"strings"

	"practices/admission-prac/pkg/clientset"
	"practices/admission-prac/pkg/handler"
	"practices/admission-prac/pkg/mutatingwebhookconfiguration"
	"practices/admission-prac/pkg/service"

	"github.com/sirupsen/logrus"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var (
	noSelfRegister        = flag.Bool("noselfregister", false, "no selfregister")
	logLevel              = flag.Int("v", 4 /*Log Info*/, "number for the log level verbosity")
	mutatePath            = flag.String("mutatepath", "/mutate", "mutate path")
	failFailurePolicy     = "Fail"
	failurePolicy         = flag.String("failurepolicy", failFailurePolicy, "failure policy")
	defaultNamespaceLabel = "test-webhook"
	namespaceLabel        = flag.String("namespacelabel", defaultNamespaceLabel, "label of namespace to apply webhook")
	serviceSelectorKey    = flag.String("serviceselectorkey", "app", "service selector key")
	serviceSelectorValue  = flag.String("serviceselectorvalue", "test-mutate-webhook", "service selector value")
	servicePort           = flag.Int("serviceport", 443, "port of service")
	targetPortName        = flag.String("targetportname", "admission-api", "name of targetport")
	webhookConfigName     = flag.String("webhookconfigname", "test-admission-mutate", "name of mutatewebhookconfiguration")
	webhookName           = flag.String("webhookname", "test-mutate-webhook.noorganization.io", "name of mutating admission webhook")
)

type SelfRegisterParameters struct {
	ServiceName      string
	ServiceNamespace string
}

func main() {
	flag.Parse()
	setupLogging()
	logrus.Println("starting")
	mux := http.NewServeMux()
	mux.Handle(*mutatePath, handler.NewMutateHandler())
	server := http.Server{
		Handler: mux,
		Addr:    ":18443",
	}

	clientset.InitClientset()
	stopCh := make(chan struct{})
	defer close(stopCh)
	go handler.StartInformer(stopCh)

	if !*noSelfRegister {
		logrus.Println("to do self register")
		selfRegisterParameters := SelfRegisterParameters{
			ServiceName:      "test-mutate-webhook",
			ServiceNamespace: "test",
		}
		go selfRegister(selfRegisterParameters)
	}

	logrus.Fatal(server.ListenAndServe())
	logrus.Println("exiting")
}

func selfRegister(parameters SelfRegisterParameters) {
	logrus.Println("self registering")
	// clientset.InitClientset()
	serviceParameters := service.ServiceParameters{
		Name:      parameters.ServiceName,
		Namespace: parameters.ServiceNamespace,
		Selector: map[string]string{
			*serviceSelectorKey: *serviceSelectorValue,
		},
		Ports: []corev1.ServicePort{
			{
				Port: int32(*servicePort),
				TargetPort: intstr.IntOrString{
					Type:   intstr.String,
					StrVal: *targetPortName,
				},
			},
		},
	}
	service.CreateService(serviceParameters)

	mutatingWebhookConfigurationParameters := mutatingwebhookconfiguration.MutatingWebhookConfigurationParameters{
		ConfigurationName: *webhookConfigName,
		WebhookName:       *webhookName,
		ServiceReference: admissionregistrationv1.ServiceReference{
			Name:      parameters.ServiceName,
			Namespace: parameters.ServiceNamespace,
			Path:      mutatePath,
		},
		WebhookNamespaceSelector: metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      *namespaceLabel,
					Operator: metav1.LabelSelectorOpExists,
				},
			},
		},
	}
	if *failurePolicy == failFailurePolicy {
		mutatingWebhookConfigurationParameters.FailurePolicy = admissionregistrationv1.FailurePolicyType(admissionregistrationv1.Fail)
	}
	mutatingwebhookconfiguration.CreateMutateWebhookConfiguration(mutatingWebhookConfigurationParameters)
}

func setupLogging() {
	// parse log level(default level: info)
	var level logrus.Level
	if *logLevel >= int(logrus.TraceLevel) {
		level = logrus.TraceLevel
	} else if *logLevel <= int(logrus.PanicLevel) {
		level = logrus.PanicLevel
	} else {
		level = logrus.Level(*logLevel)
	}

	logrus.SetLevel(level)
	logrus.SetFormatter(&logrus.JSONFormatter{
		CallerPrettyfier: func(f *runtime.Frame) (string, string) {
			s := strings.Split(f.Function, ".")
			funcName := s[len(s)-1]
			fileName := path.Base(f.File)
			return funcName, fmt.Sprintf("%s:%d", fileName, f.Line)
		}})
	logrus.SetReportCaller(true)
}
