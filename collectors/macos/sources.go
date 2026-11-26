package macos

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"emperror.dev/errors"
	"howett.net/plist"
)

// maxManagedPreferencesSize is the largest managed preferences file read.
const maxManagedPreferencesSize = 1 << 20

type systemProfile struct {
	Hardware []hardwareOverview `json:"SPHardwareDataType"`
	Software []softwareOverview `json:"SPSoftwareDataType"`
}

type hardwareOverview struct {
	MachineName      string `json:"machine_name"`
	MachineModel     string `json:"machine_model"`
	ModelNumber      string `json:"model_number"`
	SerialNumber     string `json:"serial_number"`
	PlatformUUID     string `json:"platform_UUID"`
	ProvisioningUDID string `json:"provisioning_UDID"`
}

type softwareOverview struct {
	LocalHostName string `json:"local_host_name"`
}

func (c *collector) getSystemProfile(ctx context.Context) (*systemProfile, error) {
	output, err := c.command(ctx, "system_profiler", "SPHardwareDataType", "SPSoftwareDataType", "-json")
	if err != nil {
		return nil, errors.WrapIf(err, "could not get system profile")
	}

	profile := &systemProfile{}
	if err := json.Unmarshal(output, profile); err != nil {
		return nil, errors.WrapIf(err, "could not parse system profile")
	}

	if len(profile.Hardware) == 0 {
		return nil, errors.New("system profile contains no hardware overview")
	}

	return profile, nil
}

func (p *systemProfile) hardware() hardwareOverview {
	return p.Hardware[0]
}

func (p *systemProfile) software() softwareOverview {
	if len(p.Software) == 0 {
		return softwareOverview{}
	}

	return p.Software[0]
}

type mdmEnrollment struct {
	Enrolled   bool
	DEP        bool
	ServerHost string
}

func (c *collector) getMDMEnrollment(ctx context.Context) (mdmEnrollment, bool) {
	output, _ := c.command(ctx, "profiles", "status", "-type", "enrollment")

	return parseMDMEnrollment(output)
}

func parseMDMEnrollment(output []byte) (mdmEnrollment, bool) {
	enrollment := mdmEnrollment{}
	found := false

	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)

		switch strings.TrimSpace(key) {
		case "Enrolled via DEP":
			enrollment.DEP = strings.HasPrefix(value, "Yes")
			found = true
		case "MDM enrollment":
			enrollment.Enrolled = strings.HasPrefix(value, "Yes")
			found = true
		case "MDM server":
			enrollment.ServerHost = urlHost(value)
		}
	}

	return enrollment, found
}

// urlHost returns the host of the MDM server URL.
func urlHost(value string) string {
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}

	u, err := url.Parse(value)
	if err != nil {
		return ""
	}

	return u.Hostname()
}

type managedPreference struct {
	domain string
	key    string
	label  string
}

// managedPreferenceIsTrue reports whether the managed preferences of 'domain'
// in 'dir' set 'key' to true. A missing, unreadable or too large file, a
// missing key or a value that isn't a boolean count as false.
func managedPreferenceIsTrue(dir, domain, key string) bool {
	f, err := os.Open(filepath.Join(dir, domain+".plist"))
	if err != nil {
		return false
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, maxManagedPreferencesSize+1))
	if err != nil || len(data) > maxManagedPreferencesSize {
		return false
	}

	var prefs map[string]any
	if _, err := plist.Unmarshal(data, &prefs); err != nil {
		return false
	}

	on, _ := prefs[key].(bool)

	return on
}
