package azure

type AzureMetadataInstance struct {
	Compute AzureMetadataCompute `json:"compute,omitempty"`
	Network AzureMetadataNetwork `json:"network,omitempty"`
}

type AzureMetadataCompute struct {
	AzEnvironment              string                              `json:"azEnvironment,omitempty"`
	AdditionalCapabilities     AzureMetadataAdditionalCapabilities `json:"additionalCapabilities,omitempty"`
	CustomData                 string                              `json:"customData,omitempty"`
	EvictionPolicy             string                              `json:"evictionPolicy,omitempty"`
	ExtendedLocation           AzureMetadataExtendedLocation       `json:"extendedLocation,omitempty"`
	Host                       AzureMetadataHost                   `json:"host,omitempty"`
	HostGroup                  AzureMetadataHostGroup              `json:"hostGroup,omitempty"`
	IsHostCompatibilityLayerVM string                              `json:"isHostCompatibilityLayerVm,omitempty"`
	LicenseType                string                              `json:"licenseType,omitempty"`
	Location                   string                              `json:"location,omitempty"`
	Name                       string                              `json:"name,omitempty"`
	Offer                      string                              `json:"offer,omitempty"`
	OSProfile                  AzureMetadataOSProfile              `json:"osProfile,omitempty"`
	OsType                     string                              `json:"osType,omitempty"`
	PlacementGroupID           string                              `json:"placementGroupId,omitempty"`
	Plan                       AzureMetadataPlan                   `json:"plan,omitempty"`
	PlatformUpdateDomain       string                              `json:"platformUpdateDomain,omitempty"`
	PlatformFaultDomain        string                              `json:"platformFaultDomain,omitempty"`
	PlatformSubFaultDomain     string                              `json:"platformSubFaultDomain,omitempty"`
	Priority                   string                              `json:"priority,omitempty"`
	Provider                   string                              `json:"provider,omitempty"`
	PublicKeys                 []AzureMetadataPublicKey            `json:"publicKeys,omitempty"`
	Publisher                  string                              `json:"publisher,omitempty"`
	ResourceGroupName          string                              `json:"resourceGroupName,omitempty"`
	ResourceID                 string                              `json:"resourceId,omitempty"`
	Sku                        string                              `json:"sku,omitempty"`
	SecurityProfile            AzureMetadataSecurityProfile        `json:"securityProfile,omitempty"`
	SubscriptionID             string                              `json:"subscriptionId,omitempty"`
	Tags                       string                              `json:"tags,omitempty"`
	TagsList                   []AzureMetadataTag                  `json:"tagsList,omitempty"`
	UserData                   []byte                              `json:"userData,omitempty"`
	VirtualMachineScaleSet     AzureMetadataVirtualMachineScaleSet `json:"virtualMachineScaleSet,omitempty"`
	Version                    string                              `json:"version,omitempty"`
	VMID                       string                              `json:"vmId,omitempty"`
	VMScaleSetName             string                              `json:"vmScaleSetName,omitempty"`
	VMSize                     string                              `json:"vmSize,omitempty"`
	Zone                       string                              `json:"zone,omitempty"`
}

type AzureMetadataAdditionalCapabilities struct {
	HibernationEnabled string `json:"hibernationEnabled,omitempty"`
}

type AzureMetadataExtendedLocation struct {
	Type string `json:"type,omitempty"`
	Name string `json:"name,omitempty"`
}

type AzureMetadataHost struct {
	ID string `json:"id,omitempty"`
}

type AzureMetadataHostGroup struct {
	ID string `json:"id,omitempty"`
}

type AzureMetadataOSProfile struct {
	AdminUsername                 string `json:"adminUsername,omitempty"`
	ComputerName                  string `json:"computerName,omitempty"`
	DisablePasswordAuthentication string `json:"disablePasswordAuthentication,omitempty"`
}

type AzureMetadataPlan struct {
	Name          string `json:"name,omitempty"`
	Product       string `json:"product,omitempty"`
	PromotionCode string `json:"promotionCode,omitempty"`
	Publisher     string `json:"publisher,omitempty"`
}

type AzureMetadataPublicKey struct {
	KeyData string `json:"keyData,omitempty"`
	Path    string `json:"path,omitempty"`
}

type AzureMetadataSecurityProfile struct {
	SecureBootEnabled string `json:"secureBootEnabled,omitempty"`
	VirtualTpmEnabled string `json:"virtualTpmEnabled,omitempty"`
	EncryptionAtHost  string `json:"encryptionAtHost,omitempty"`
	SecurityType      string `json:"securityType,omitempty"`
}

type AzureMetadataTag struct {
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}

type AzureMetadataVirtualMachineScaleSet struct {
	ID string `json:"id,omitempty"`
}

type AzureMetadataNetwork struct {
	Interface []AzureMetadataNetworkInterface `json:"interface,omitempty"`
}

type AzureMetadataNetworkInterface struct {
	IPv4       AzureMetadataIPv4 `json:"iPv4,omitempty"`
	IPv6       AzureMetadataIPv6 `json:"iPv6,omitempty"`
	MacAddress string            `json:"macAddress,omitempty"`
}

type AzureMetadataIPv4 struct {
	IPAddress []AzureMetadataIPAddress `json:"ipAddress,omitempty"`
	Subnet    []AzureMetadataSubnet    `json:"subnet,omitempty"`
}

type AzureMetadataIPv6 struct {
	IPAddress []AzureMetadataIPAddress `json:"ipAddress,omitempty"`
}

type AzureMetadataIPAddress struct {
	PrivateIpAddress string `json:"privateIpAddress,omitempty"`
	PublicIpAddress  string `json:"publicIpAddress,omitempty"`
}

type AzureMetadataSubnet struct {
	Address string `json:"address,omitempty"`
	Prefix  string `json:"prefix,omitempty"`
}

type AzureMetadataLoadBalancer struct {
	LoadBalancer AzureMetadataLB `json:"loadbalancer,omitempty"`
}

type AzureMetadataLB struct {
	PublicIPAddresses []AzureMetadataLoadBalancerPublicIPAddress `json:"publicIPAddresses,omitempty"`
}

type AzureMetadataLoadBalancerPublicIPAddress struct {
	FrontendIpAddress string `json:"frontendIpAddress,omitempty"`
	PrivateIpAddress  string `json:"privateIpAddress,omitempty"`
}
