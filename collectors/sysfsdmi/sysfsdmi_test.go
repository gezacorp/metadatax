package sysfsdmi_test

import (
	"context"
	"testing"

	"github.com/gohobby/assert"

	"github.com/gezacorp/metadatax/collectors/sysfsdmi"
)

type mdgetter struct{}

func (g *mdgetter) GetContent(key string) string {
	data := map[string]string{
		"bios_date":         "03/01/2023",
		"bios_release":      "0.0",
		"bios_vendor":       "EDK II",
		"bios_version":      "edk2-stable202302-for-qemu",
		"chassis_asset_tag": "chassisassettag",
		"chassis_serial":    "chassisserial",
		"chassis_type":      "1",
		"chassis_vendor":    "QEMU",
		"chassis_version":   "pc-q35-8.1",
		"product_family":    "productfamily",
		"product_name":      "Standard PC (Q35 + ICH9, 2009)",
		"product_serial":    "QEMU-0B58FE24533B92D6E93F6607F5E0BF3C",
		"product_sku":       "QEMU-SKU-0B58FE24533B92D6E93F6607F5E0BF3C",
		"product_version":   "pc-q35-8.1",
		"sys_vendor":        "QEMU",
	}

	return data[key]
}

func TestGetMetadata(t *testing.T) {
	t.Parallel()

	expectedLabels := map[string][]string{
		"sysfsdmi:bios:date":         {"03/01/2023"},
		"sysfsdmi:bios:release":      {"0.0"},
		"sysfsdmi:bios:vendor":       {"EDK II"},
		"sysfsdmi:bios:version":      {"edk2-stable202302-for-qemu"},
		"sysfsdmi:chassis:asset-tag": {"chassisassettag"},
		"sysfsdmi:chassis:serial":    {"chassisserial"},
		"sysfsdmi:chassis:type":      {"1"},
		"sysfsdmi:chassis:vendor":    {"QEMU"},
		"sysfsdmi:chassis:version":   {"pc-q35-8.1"},
		"sysfsdmi:product:family":    {"productfamily"},
		"sysfsdmi:product:name":      {"Standard PC (Q35 + ICH9, 2009)"},
		"sysfsdmi:product:serial":    {"QEMU-0B58FE24533B92D6E93F6607F5E0BF3C"},
		"sysfsdmi:product:sku":       {"QEMU-SKU-0B58FE24533B92D6E93F6607F5E0BF3C"},
		"sysfsdmi:product:version":   {"pc-q35-8.1"},
	}

	collector := sysfsdmi.New(
		sysfsdmi.CollectorWithMetadataGetter(&mdgetter{}),
		sysfsdmi.CollectorWithForceHasSysFS(),
	)
	md, err := collector.GetMetadata(context.Background())
	assert.Nil(t, err)

	assert.Equal(t, expectedLabels, map[string][]string(md.GetLabels()))
}
