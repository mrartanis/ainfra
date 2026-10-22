package apiclient_test

import (
	"ainfra/internal/apiclient"
	"testing"
)

func TestLocalMetricsURLOnlyAllowsExplicitLoopbackEndpoint(t *testing.T) {
	for _, url := range []string{"http://127.0.0.1:9080/metrics", "http://[::1]:9080/metrics"} {
		if err := apiclient.ValidateLocalMetricsURL(url); err != nil {
			t.Fatal(url, err)
		}
	}
	for _, url := range []string{"http://localhost:9080/metrics", "http://192.168.1.1:80/metrics", "https://127.0.0.1:9080/metrics", "http://user@127.0.0.1:9080/metrics", "http://127.0.0.1/metrics", "http://127.0.0.1:9080/other", "http://127.0.0.1:9080/metrics?url=external", "http://127.0.0.1:70000/metrics", "http://127.0.0.1:9080/metrics#secret"} {
		if err := apiclient.ValidateLocalMetricsURL(url); err == nil {
			t.Fatal("accepted", url)
		}
	}
}
