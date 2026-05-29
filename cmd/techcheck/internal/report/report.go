package report

import (
	"time"

	"github.com/google/uuid"
)

const (
	Schema      = "techcheck.v1"
	ToolVersion = "0.1.10"
	AppID       = "sunrise-techcheck"
)

// NetworkAdapterType values. Stored as strings on the wire per PRD §7.4.
const (
	NetworkAdapterWired    = "wired"
	NetworkAdapterWireless = "wireless"
)

// LatencyMethod values — which measurement technique the tool actually used.
const (
	LatencyMethodICMP = "icmp"
	LatencyMethodTCP  = "tcp"
)

// MTUStatus values.
const (
	MTUStatusPass       = "pass"
	MTUStatusFragmented = "fragmented"
	MTUStatusBlackHole  = "black-hole"
)

// ReachabilityStatus values.
const (
	ReachabilityReachable = "reachable"
	ReachabilityBlocked   = "blocked"
)

// Report is the top-level structure matching PRD §7.4.
type Report struct {
	Schema         string    `json:"schema"`
	ToolVersion    string    `json:"tool_version"`
	AppID          string    `json:"app_id"`
	RunID          string    `json:"run_id"`
	RunStartedAt   time.Time `json:"run_started_at"`
	RunCompletedAt time.Time `json:"run_completed_at"`
	Consent        Consent   `json:"consent"`
	User           User      `json:"user"`
	Machine        Machine   `json:"machine"`
	Network        Network   `json:"network"`
	Errors         []Error   `json:"errors"`
}

type Consent struct {
	Accepted bool      `json:"accepted"`
	At       time.Time `json:"at"`
}

type User struct {
	FullName string `json:"full_name"`
	Email    string `json:"email"`
}

type Machine struct {
	OS               OS               `json:"os"`
	MachineID        string           `json:"machine_id"`
	Hostname         string           `json:"hostname"`
	LoggedInUsername string           `json:"logged_in_username"`
	Locale           string           `json:"locale"`
	Timezone         string           `json:"timezone"`
	CPU              CPU              `json:"cpu"`
	Memory           Memory           `json:"memory"`
	Storage          Storage          `json:"storage"`
	GPUs             []GPU            `json:"gpus"`
	Displays         []Display        `json:"displays"`
	Peripherals      Peripherals      `json:"peripherals"`
	Virtualisation   Virtualisation   `json:"virtualisation"`
	Power            Power            `json:"power"`
	NetworkAdapters  []NetworkAdapter `json:"network_adapters"`
	Security         Security         `json:"security"`
}

type OS struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Build   string `json:"build"`
	Arch    string `json:"arch"`
}

type CPU struct {
	Model            string `json:"model"`
	PhysicalCores    int    `json:"physical_cores"`
	LogicalCores     int    `json:"logical_cores"`
	BaseFrequencyMHz int    `json:"base_frequency_mhz"`
}

type Memory struct {
	TotalBytes uint64 `json:"total_bytes"`
}

type Storage struct {
	SystemVolumeFreeBytes uint64 `json:"system_volume_free_bytes"`
}

type GPU struct {
	Model     string `json:"model"`
	VRAMBytes uint64 `json:"vram_bytes"`
}

type Display struct {
	WidthPX   int `json:"width_px"`
	HeightPX  int `json:"height_px"`
	RefreshHz int `json:"refresh_hz"`
}

type Peripherals struct {
	WebcamPresent      bool        `json:"webcam_present"`
	MicPresent         bool        `json:"mic_present"`
	DefaultAudioDevice string      `json:"default_audio_device"`
	USBDeviceCount     int         `json:"usb_device_count"`
	USBDevices         []USBDevice `json:"usb_devices"`
}

// USBDevice describes a single attached USB peripheral. Serial numbers are
// intentionally omitted — they're a stable hardware identifier beyond
// machine_id and aren't needed for an IT tech check.
type USBDevice struct {
	Name         string `json:"name"`
	Manufacturer string `json:"manufacturer"`
	VendorID     string `json:"vendor_id"`  // "0x05ac"
	ProductID    string `json:"product_id"` // "0x8600"
	Speed        string `json:"speed"`      // "Up to 480 Mb/sec"
}

type Virtualisation struct {
	IsVM       bool   `json:"is_vm"`
	Hypervisor string `json:"hypervisor"`
}

type Power struct {
	OnAC           bool `json:"on_ac"`
	BatteryPercent int  `json:"battery_percent"`
}

type NetworkAdapter struct {
	Type          string `json:"type"` // "wired" | "wireless"
	LinkSpeedMbps int    `json:"link_speed_mbps"`
	IsDefault     bool   `json:"is_default"`
	WifiSSID      string `json:"wifi_ssid,omitempty"`
}

type Security struct {
	AVProduct       string            `json:"av_product"`
	FirewallEnabled bool              `json:"firewall_enabled"`
	VPNDetected     []string          `json:"vpn_detected"`
	SystemProxy     map[string]string `json:"system_proxy"`
}

type Network struct {
	PublicIP    string        `json:"public_ip"`
	DetectedGeo string        `json:"detected_geo"`
	ClientGeo   ClientGeo     `json:"client_geo"`
	POPs        []POPResult   `json:"pops"`
}

// ClientGeo is the client-side geolocation based on the user's public IP,
// looked up via ipinfo.io. All fields zero-value on lookup failure.
type ClientGeo struct {
	City    string  `json:"city"`
	Region  string  `json:"region"`
	Country string  `json:"country"` // ISO 3166-1 alpha-2
	Lat     float64 `json:"lat"`
	Lon     float64 `json:"lon"`
	ISP     string  `json:"isp"`
}

type POPResult struct {
	ID          string        `json:"id"`
	RegionLabel string        `json:"region_label"`
	Latency     LatencyResult `json:"latency"`
	Throughput  Throughput    `json:"throughput"`
	Jitter      Jitter        `json:"jitter"`
	Loss        Loss          `json:"loss"`
	MTU         MTUResult     `json:"mtu"`
	ClockSkewMS int64         `json:"clock_skew_ms"`
	UDP4172     Reachability  `json:"udp_4172"`
	TCP443      Reachability  `json:"tcp_443"`
	DNSMS       int64         `json:"dns_ms"`
}

type LatencyResult struct {
	Method   string  `json:"method"` // "icmp" | "tcp"
	MinMS    float64 `json:"min_ms"`
	MedianMS float64 `json:"median_ms"`
	P95MS    float64 `json:"p95_ms"`
	MaxMS    float64 `json:"max_ms"`
	StddevMS float64 `json:"stddev_ms"`
}

type Throughput struct {
	DownMbps float64 `json:"down_mbps"`
	UpMbps   float64 `json:"up_mbps"`
}

type Jitter struct {
	VarianceMS float64 `json:"variance_ms"`
}

type Loss struct {
	Pct float64 `json:"pct"`
}

type MTUResult struct {
	Status string `json:"status"` // "pass" | "fragmented" | "black-hole"
}

type Reachability struct {
	Status string `json:"status"` // "reachable" | "blocked"
}

type Error struct {
	POPID   string `json:"pop_id,omitempty"`
	Test    string `json:"test"`
	Message string `json:"message"`
}

// New returns a fresh Report with Schema, ToolVersion, RunID, and
// RunStartedAt populated, and initialised collections so JSON never contains
// null slices.
func New() *Report {
	return &Report{
		Schema:      Schema,
		ToolVersion: ToolVersion,
		AppID:       AppID,
		RunID:       uuid.NewString(),
		RunStartedAt:  time.Now().UTC(),
		Errors:        []Error{},
		Machine: Machine{
			GPUs:            []GPU{},
			Displays:        []Display{},
			NetworkAdapters: []NetworkAdapter{},
			Peripherals: Peripherals{
				USBDevices: []USBDevice{},
			},
			Security: Security{
				VPNDetected: []string{},
				SystemProxy: map[string]string{},
			},
		},
		Network: Network{
			POPs: []POPResult{},
		},
	}
}

// RecordError appends a per-test failure to r.Errors. popID may be empty for
// errors not attributable to a specific POP (e.g. sysinfo collection).
func (r *Report) RecordError(popID, test, message string) {
	r.Errors = append(r.Errors, Error{POPID: popID, Test: test, Message: message})
}
