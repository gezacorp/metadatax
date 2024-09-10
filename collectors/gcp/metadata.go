package gcp

type GCPMetadataInstance struct {
	Attributes        map[string]string                    `json:"attributes,omitempty"`
	PartnerAttributes map[string]string                    `json:"partnerAttributes,omitempty"`
	GuestAttributes   map[string]string                    `json:"guestAttributes,omitempty"`
	CPUPlatform       string                               `json:"cpuPlatform,omitempty"`
	Description       string                               `json:"description,omitempty"`
	Disks             []GCPMetadataDisk                    `json:"disks,omitempty"`
	Hostname          string                               `json:"hostname,omitempty"`
	ID                int64                                `json:"id,omitempty"`
	Image             string                               `json:"image,omitempty"`
	Licenses          []GCPMetadataLicense                 `json:"licenses,omitempty"`
	MachineType       string                               `json:"machineType,omitempty"`
	MaintenanceEvent  string                               `json:"maintenanceEvent,omitempty"`
	Name              string                               `json:"name,omitempty"`
	NetworkInterfaces []GCPMetadataNetworkInterface        `json:"networkInterfaces,omitempty"`
	Preempted         string                               `json:"preempted,omitempty"`
	RemainingCPUTime  int                                  `json:"remainingCpuTime,omitempty"`
	Scheduling        GCPMetadataScheduling                `json:"scheduling,omitempty"`
	ServiceAccounts   map[string]GCPMetadataServiceAccount `json:"serviceAccounts,omitempty"`
	Tags              []string                             `json:"tags,omitempty"`
	VirtualClock      GCPMetadataVirtualClock              `json:"virtualClock,omitempty"`
	Zone              string                               `json:"zone,omitempty"`
}

type GCPMetadataDisk struct {
	DeviceName string `json:"deviceName,omitempty"`
	Index      int    `json:"index,omitempty"`
	Interface  string `json:"interface,omitempty"`
	Mode       string `json:"mode,omitempty"`
	Type       string `json:"type,omitempty"`
}

type GCPMetadataLicense struct {
	ID string `json:"id,omitempty"`
}

type GCPMetadataAccessConfig struct {
	ExternalIP string `json:"externalIp,omitempty"`
	Type       string `json:"type,omitempty"`
}

type GCPMetadataNetworkInterface struct {
	AccessConfigs     []GCPMetadataAccessConfig `json:"accessConfigs,omitempty"`
	DNSServers        []string                  `json:"dnsServers,omitempty"`
	ForwardedIps      []string                  `json:"forwardedIps,omitempty"`
	Gateway           string                    `json:"gateway,omitempty"`
	IP                string                    `json:"ip,omitempty"`
	IPAliases         []string                  `json:"ipAliases,omitempty"`
	Mac               string                    `json:"mac,omitempty"`
	Mtu               int                       `json:"mtu,omitempty"`
	Network           string                    `json:"network,omitempty"`
	Subnetmask        string                    `json:"subnetmask,omitempty"`
	TargetInstanceIps []string                  `json:"targetInstanceIps,omitempty"`
}

type GCPMetadataScheduling struct {
	AutomaticRestart  string `json:"automaticRestart,omitempty"`
	OnHostMaintenance string `json:"onHostMaintenance,omitempty"`
	Preemptible       string `json:"preemptible,omitempty"`
}

type GCPMetadataServiceAccount struct {
	Aliases []string `json:"aliases,omitempty"`
	Email   string   `json:"email,omitempty"`
	Scopes  []string `json:"scopes,omitempty"`
}

type GCPMetadataVirtualClock struct {
	DriftToken string `json:"driftToken,omitempty"`
}

type GCPProjectMetadata struct {
	ID         string            `json:"projectId,omitempty"`
	NumericID  int64             `json:"numericProjectId,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
}
