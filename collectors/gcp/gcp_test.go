package gcp_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/gohobby/assert"

	"github.com/gezacorp/metadatax/collectors/gcp"
)

type mdgetter struct{}

func (g *mdgetter) GetInstanceMetadata(ctx context.Context) (*gcp.GCPMetadataInstance, error) {
	file, err := os.Open("testdata/instance.json")
	if err != nil {
		return nil, err
	}

	content, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	var md gcp.GCPMetadataInstance
	err = json.Unmarshal(content, &md)
	if err != nil {
		return nil, err
	}

	return &md, nil
}

func TestGetMetadata(t *testing.T) {
	t.Parallel()

	expectedLabels := map[string][]string{
		"gcp:attributes:mdkey":             {"mdvalue"},
		"gcp:cpu-platform":                 {"AMD Rome"},
		"gcp:id":                           {"5240495278393851000"},
		"gcp:image:name":                   {"debian-11-bullseye-v20231115"},
		"gcp:image:project":                {"debian-cloud"},
		"gcp:machine:project":              {"758913618900"},
		"gcp:machine:type":                 {"e2-medium"},
		"gcp:name":                         {"instance-1"},
		"gcp:network:mac":                  {"42:01:0a:a4:00:02"},
		"gcp:network:private-ipv4":         {"10.164.0.2"},
		"gcp:network:public-ipv4":          {"35.204.15.15"},
		"gcp:placement:project":            {"758913618900"},
		"gcp:placement:region":             {"europe-west4"},
		"gcp:placement:zone":               {"europe-west4-a"},
		"gcp:scheduling:automatic-restart": {"true"},
		"gcp:scheduling:onHostMaintenance": {"migrate"},
		"gcp:scheduling:preemptible":       {"false"},
		"gcp:serviceaccount:758913618900-compute@developer.gserviceaccount.com:alias": {"default"},
		"gcp:serviceaccount:758913618900-compute@developer.gserviceaccount.com:email": {
			"758913618900-compute@developer.gserviceaccount.com",
		},
		"gcp:serviceaccount:758913618900-compute@developer.gserviceaccount.com:scope": {
			"https://www.googleapis.com/auth/devstorage.read_only",
			"https://www.googleapis.com/auth/logging.write",
			"https://www.googleapis.com/auth/monitoring.write",
			"https://www.googleapis.com/auth/servicecontrol",
			"https://www.googleapis.com/auth/service.management.readonly",
			"https://www.googleapis.com/auth/trace.append",
		},
		"gcp:serviceaccount:default:alias": {"default"},
		"gcp:serviceaccount:default:email": {
			"758913618900-compute@developer.gserviceaccount.com",
		},
		"gcp:serviceaccount:default:scope": {
			"https://www.googleapis.com/auth/devstorage.read_only",
			"https://www.googleapis.com/auth/logging.write",
			"https://www.googleapis.com/auth/monitoring.write",
			"https://www.googleapis.com/auth/servicecontrol",
			"https://www.googleapis.com/auth/service.management.readonly",
			"https://www.googleapis.com/auth/trace.append",
		},
		"gcp:tag": {
			"tag-1",
			"tag-2",
		},
	}

	collector := gcp.New(
		gcp.CollectorWithGCPMetadataClient(&mdgetter{}),
		gcp.CollectorWithForceOnGoogle(),
	)
	md, err := collector.GetMetadata(context.Background())
	assert.Nil(t, err)

	assert.Equal(t, expectedLabels, map[string][]string(md.GetLabels()))
}
