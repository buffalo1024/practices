package config

var (
	namespace   = "test"
	serviceName = "test-mutate-webhook"
)

func SetConfig(namespaceToSet, serviceNameToSet string) {
	namespace = namespaceToSet
	serviceName = serviceNameToSet
}

func GetNamespace() string {
	return namespace
}

func GetServiceName() string {
	return serviceName
}
