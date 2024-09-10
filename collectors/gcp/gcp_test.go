package gcp_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"

	"emperror.dev/errors"
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

	md := &gcp.GCPMetadataInstance{}
	err = json.Unmarshal(content, md)
	if err != nil {
		return nil, err
	}

	return md, nil
}

func (g *mdgetter) GetProjectMetadata(ctx context.Context) (*gcp.GCPProjectMetadata, error) {
	file, err := os.Open("testdata/project.json")
	if err != nil {
		return nil, err
	}

	content, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	md := &gcp.GCPProjectMetadata{}
	err = json.Unmarshal(content, md)
	if err != nil {
		return nil, err
	}

	return md, nil
}

func (g *mdgetter) GetInstanceIdentityToken(ctx context.Context, audience string, format string) ([]byte, error) {
	return nil, errors.NewPlain("GetInstanceIdentityToken is not implemented")
}

func TestGetMetadata(t *testing.T) {
	t.Parallel()

	expectedLabels := map[string][]string{
		"gcp:instance:attribute:mdkey":              {"mdvalue"},
		"gcp:instance:cpu-platform":                 {"AMD Rome"},
		"gcp:instance:id":                           {"5240495278393851000"},
		"gcp:instance:image:name":                   {"debian-11-bullseye-v20231115"},
		"gcp:instance:image:project":                {"debian-cloud"},
		"gcp:instance:machine:project":              {"758913618900"},
		"gcp:instance:machine:type":                 {"e2-medium"},
		"gcp:instance:name":                         {"instance-1"},
		"gcp:instance:network:mac":                  {"42:01:0a:a4:00:02"},
		"gcp:instance:network:private-ipv4":         {"10.164.0.2"},
		"gcp:instance:network:public-ipv4":          {"35.204.15.15"},
		"gcp:instance:placement:project":            {"758913618900"},
		"gcp:instance:placement:region":             {"europe-west4"},
		"gcp:instance:placement:zone":               {"europe-west4-a"},
		"gcp:instance:scheduling:automatic-restart": {"true"},
		"gcp:instance:scheduling:onHostMaintenance": {"migrate"},
		"gcp:instance:scheduling:preemptible":       {"false"},
		"gcp:instance:serviceaccount:758913618900-compute@developer.gserviceaccount.com:alias": {"default"},
		"gcp:instance:serviceaccount:758913618900-compute@developer.gserviceaccount.com:email": {
			"758913618900-compute@developer.gserviceaccount.com",
		},
		"gcp:instance:serviceaccount:758913618900-compute@developer.gserviceaccount.com:scope": {
			"https://www.googleapis.com/auth/devstorage.read_only",
			"https://www.googleapis.com/auth/logging.write",
			"https://www.googleapis.com/auth/monitoring.write",
			"https://www.googleapis.com/auth/servicecontrol",
			"https://www.googleapis.com/auth/service.management.readonly",
			"https://www.googleapis.com/auth/trace.append",
		},
		"gcp:instance:serviceaccount:default:alias": {"default"},
		"gcp:instance:serviceaccount:default:email": {
			"758913618900-compute@developer.gserviceaccount.com",
		},
		"gcp:instance:serviceaccount:default:scope": {
			"https://www.googleapis.com/auth/devstorage.read_only",
			"https://www.googleapis.com/auth/logging.write",
			"https://www.googleapis.com/auth/monitoring.write",
			"https://www.googleapis.com/auth/servicecontrol",
			"https://www.googleapis.com/auth/service.management.readonly",
			"https://www.googleapis.com/auth/trace.append",
		},
		"gcp:instance:tag": {
			"tag-1",
			"tag-2",
		},
		"gcp:project:attribute:joska": {
			"pista",
		},
		"gcp:project:id": {
			"inlaid-fuze-402617",
		},
		"gcp:project:id:numeric": {
			"758913618900",
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
