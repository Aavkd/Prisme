package edge

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestConfigurationSecretsStayOutOfPublicInventory(t *testing.T) {
	source := "pilot.test {\n header_up Authorization Basic fixture-credential\n reverse_proxy app:8080\n}\n"
	if strings.Contains(publicSource(source), "fixture-credential") || !strings.Contains(publicSource(source), "app:8080") {
		t.Fatal("source redaction")
	}
	active := map[string]any{"headers": map[string]any{"Authorization": []any{"Basic fixture-credential"}}, "upstreams": []any{map[string]any{"dial": "app:8080"}}, "password": "fixture-password"}
	b, _ := json.Marshal(publicConfig(active))
	if strings.Contains(string(b), "fixture-") || !strings.Contains(string(b), "app:8080") {
		t.Fatal("active configuration redaction")
	}
}
