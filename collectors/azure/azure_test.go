package azure_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/gohobby/assert"

	"github.com/gezacorp/metadatax/collectors/azure"
)

type mdgetter struct{}

func (g *mdgetter) GetInstanceMetadata(ctx context.Context) (*azure.AzureMetadataInstance, error) {
	file, err := os.Open("testdata/instance.json")
	if err != nil {
		return nil, err
	}

	content, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	var md azure.AzureMetadataInstance
	err = json.Unmarshal(content, &md)
	if err != nil {
		return nil, err
	}

	return &md, nil
}

func (g *mdgetter) GetLoadBalancerMetadata(ctx context.Context) (*azure.AzureMetadataLoadBalancer, error) {
	file, err := os.Open("testdata/loadbalancer.json")
	if err != nil {
		return nil, err
	}

	content, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	var lb azure.AzureMetadataLoadBalancer
	err = json.Unmarshal(content, &lb)
	if err != nil {
		return nil, err
	}

	return &lb, nil
}

func TestGetMetadata(t *testing.T) {
	t.Parallel()

	expectedLabels := map[string][]string{
		"azure:name":                 {"demo"},
		"azure:network:mac":          {"000D3A27CA60"},
		"azure:network:private-ipv4": {"10.0.0.4"},
		"azure:network:public-ipv4":  {"51.0.0.1"},
		"azure:ostype":               {"Linux"},
		"azure:placement:location":   {"westeurope"},
		"azure:placement:zone":       {"1"},
		"azure:priority":             {"Spot"},
		"azure:provider":             {"Microsoft.Compute"},
		"azure:resourcegroup:name":   {"base"},
		"azure:sku":                  {"22_04-lts-gen2"},
		"azure:subscription:id":      {"aef37fca-5441-4532-a1a9-726b55173ca0"},
		"azure:tag:bela":             {"geza"},
		"azure:tag:joska":            {"pista"},
		"azure:vm:id":                {"afe12e91-33d9-4b5b-b915-ac81fe117b12"},
		"azure:vm:offer":             {"0001-com-ubuntu-server-jammy"},
		"azure:vm:publisher":         {"canonical"},
		"azure:vm:scaleset:name":     {"default"},
		"azure:vm:size":              {"Standard_B2ats_v2"},
		"azure:vm:version":           {"22.04.202311010"},
	}

	collector := azure.New(
		azure.CollectorWithMetadataGetter(&mdgetter{}),
		azure.CollectorWithForceOnAzure(),
	)
	md, err := collector.GetMetadata(context.Background())
	assert.Nil(t, err)

	assert.Equal(t, expectedLabels, map[string][]string(md.GetLabels()))
}
