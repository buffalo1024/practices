package mutatingwebhookconfiguration

import (
	"context"
	"log"

	"practices/admission-prac/pkg/clientset"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type MutatingWebhookConfigurationParameters struct {
	ConfigurationName string
	WebhookName       string
	admissionregistrationv1.ServiceReference
	FailurePolicy            admissionregistrationv1.FailurePolicyType
	WebhookNamespaceSelector v1.LabelSelector
}

func CreateMutateWebhookConfiguration(parameters MutatingWebhookConfigurationParameters) {
	cfg := admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: v1.ObjectMeta{
			Name: parameters.ConfigurationName,
		},
		Webhooks: []admissionregistrationv1.MutatingWebhook{
			{
				Name: parameters.WebhookName,
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					Service: &parameters.ServiceReference,
				},
				Rules: []admissionregistrationv1.RuleWithOperations{
					{
						Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create},
						Rule: admissionregistrationv1.Rule{
							APIGroups:   []string{"apps", ""},
							APIVersions: []string{"v1"},
							Resources:   []string{"pods"},
						},
					},
				},
				FailurePolicy:           &parameters.FailurePolicy,
				NamespaceSelector:       &parameters.WebhookNamespaceSelector,
				AdmissionReviewVersions: []string{"v1", "v1beta1"},
				SideEffects: func() *admissionregistrationv1.SideEffectClass {
					se := admissionregistrationv1.SideEffectClassNone
					return &se
				}(),
			},
		},
	}

	mutateAdmissionClient := clientset.GetClientset().AdmissionregistrationV1().MutatingWebhookConfigurations()
	_, err := mutateAdmissionClient.Create(context.TODO(), &cfg, v1.CreateOptions{})
	if err != nil {
		log.Printf("create mutatingwebhookconfiguration err: %v", err)
		return
	}
}
