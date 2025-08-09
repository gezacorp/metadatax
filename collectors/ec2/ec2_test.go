package ec2_test

import (
	"context"
	"testing"

	"emperror.dev/errors"
	"github.com/stretchr/testify/assert"

	"github.com/gezacorp/metadatax/collectors/ec2"
)

type imdsClient struct {
	data map[string]string
}

func (c *imdsClient) GetMetadataContent(ctx context.Context, path string) string {
	return c.data[path]
}

func (c *imdsClient) GetDynamicMetadataContent(ctx context.Context, path string) ([]byte, error) {
	return nil, errors.NewPlain("GetDynamicMetadataContent is not implemented")
}

func TestGetMetadata(t *testing.T) {
	t.Parallel()

	data := map[string]string{
		"ami-id":                         "ami-06dd92ecc74fdfb36",
		"instance-id":                    "i-0214fc003bc83bcc1",
		"instance-type":                  "t2.medium",
		"hostname":                       "ip-172-31-19-35.eu-central-1.compute.internal",
		"local-hostname":                 "ip-172-31-19-35.eu-central-1.compute.internal",
		"local-ipv4":                     "172.31.19.35",
		"mac":                            "02:80:ac:db:6e:fd",
		"public-hostname":                "ec2-18-197-158-100.eu-central-1.compute.amazonaws.com",
		"public-ipv4":                    "18.197.158.100",
		"placement/availability-zone":    "eu-central-1a",
		"placement/availability-zone-id": "euc1-az2",
		"placement/region":               "eu-central-1",
		"security-groups":                "launch-wizard-24",
		"services/domain":                "amazonaws.com",
		"services/partition":             "aws",
	}

	collector := ec2.New(
		ec2.WithIMDSClient(&imdsClient{
			data: data,
		}),
		ec2.WithForceOnEC2(),
	)

	expectedLabels := map[string][]string{
		"ec2:ami:id":                         {data["ami-id"]},
		"ec2:instance:id":                    {data["instance-id"]},
		"ec2:instance:type":                  {data["instance-type"]},
		"ec2:network:hostname":               {data["hostname"]},
		"ec2:network:local-hostname":         {data["local-hostname"]},
		"ec2:network:local-ipv4":             {data["local-ipv4"]},
		"ec2:network:mac":                    {data["mac"]},
		"ec2:network:public-hostname":        {data["public-hostname"]},
		"ec2:network:public-ipv4":            {data["public-ipv4"]},
		"ec2:placement:availability-zone":    {data["placement/availability-zone"]},
		"ec2:placement:availability-zone-id": {data["placement/availability-zone-id"]},
		"ec2:placement:region":               {data["placement/region"]},
		"ec2:security-groups":                {data["security-groups"]},
		"ec2:services:domain":                {data["services/domain"]},
		"ec2:services:partition":             {data["services/partition"]},
	}

	md, err := collector.GetMetadata(context.Background())
	assert.Nil(t, err)

	assert.Equal(t, expectedLabels, map[string][]string(md.GetLabels()))
}
