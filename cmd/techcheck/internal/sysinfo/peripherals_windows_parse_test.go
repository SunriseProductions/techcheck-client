package sysinfo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Sample output mirrors the JSON shape produced by the PowerShell scripts
// in peripherals_windows.go. The script forces array-shape via @(...), so
// these inputs always have arrays at GPUs/Screens.

func TestParseDisplaysAndGPUs_Laptop(t *testing.T) {
	in := []byte(`{
		"GPUs": [
			{"Name":"Intel(R) UHD Graphics","AdapterRAM":1073741824,"WidthPX":1920,"HeightPX":1080,"RefreshHz":60}
		],
		"Screens": [
			{"WidthPX":1920,"HeightPX":1080,"Primary":true}
		]
	}`)
	gpus, displays := parseDisplaysAndGPUs(in)

	assert.Len(t, gpus, 1)
	assert.Equal(t, "Intel(R) UHD Graphics", gpus[0].Model)
	assert.Equal(t, uint64(1073741824), gpus[0].VRAMBytes)

	assert.Len(t, displays, 1)
	assert.Equal(t, 1920, displays[0].WidthPX)
	assert.Equal(t, 1080, displays[0].HeightPX)
	assert.Equal(t, 60, displays[0].RefreshHz)
}

func TestParseDisplaysAndGPUs_MultiMonitorMatchedByResolution(t *testing.T) {
	// One adapter, two monitors at different resolutions. The adapter reports
	// only one current mode; the second monitor should still appear (from
	// Screen.AllScreens) but with RefreshHz 0 since no GPU matches.
	in := []byte(`{
		"GPUs": [
			{"Name":"NVIDIA RTX 3060","AdapterRAM":2147483648,"WidthPX":2560,"HeightPX":1440,"RefreshHz":144}
		],
		"Screens": [
			{"WidthPX":2560,"HeightPX":1440,"Primary":true},
			{"WidthPX":1920,"HeightPX":1080,"Primary":false}
		]
	}`)
	_, displays := parseDisplaysAndGPUs(in)

	assert.Len(t, displays, 2)
	// Primary matches the GPU mode → refresh filled.
	assert.Equal(t, 2560, displays[0].WidthPX)
	assert.Equal(t, 144, displays[0].RefreshHz)
	// Secondary has no matching GPU mode → refresh 0.
	assert.Equal(t, 1920, displays[1].WidthPX)
	assert.Equal(t, 0, displays[1].RefreshHz)
}

func TestParseDisplaysAndGPUs_TwoIdenticalScreens(t *testing.T) {
	// Dual 1920x1080 driven by one GPU. The adapter reports a single current
	// mode at 1920x1080@60. Both screens get refresh 60 because the join is
	// resolution-based and first-wins — this pins down current behaviour;
	// see parseDisplaysAndGPUs godoc for the caveat.
	in := []byte(`{
		"GPUs": [
			{"Name":"Intel UHD","AdapterRAM":1073741824,"WidthPX":1920,"HeightPX":1080,"RefreshHz":60}
		],
		"Screens": [
			{"WidthPX":1920,"HeightPX":1080,"Primary":true},
			{"WidthPX":1920,"HeightPX":1080,"Primary":false}
		]
	}`)
	_, displays := parseDisplaysAndGPUs(in)

	assert.Len(t, displays, 2)
	assert.Equal(t, 60, displays[0].RefreshHz)
	assert.Equal(t, 60, displays[1].RefreshHz)
}

func TestParseDisplaysAndGPUs_SkipsNamelessGPUAndBogusScreen(t *testing.T) {
	in := []byte(`{
		"GPUs": [
			{"Name":"","AdapterRAM":0,"WidthPX":0,"HeightPX":0,"RefreshHz":0},
			{"Name":"Microsoft Basic Display Adapter","AdapterRAM":0,"WidthPX":1024,"HeightPX":768,"RefreshHz":60}
		],
		"Screens": [
			{"WidthPX":0,"HeightPX":0,"Primary":false},
			{"WidthPX":1024,"HeightPX":768,"Primary":true}
		]
	}`)
	gpus, displays := parseDisplaysAndGPUs(in)

	assert.Len(t, gpus, 1)
	assert.Equal(t, "Microsoft Basic Display Adapter", gpus[0].Model)
	assert.Equal(t, uint64(0), gpus[0].VRAMBytes)

	assert.Len(t, displays, 1)
	assert.Equal(t, 1024, displays[0].WidthPX)
	assert.Equal(t, 60, displays[0].RefreshHz)
}

func TestParseDisplaysAndGPUs_InvalidJSON(t *testing.T) {
	gpus, displays := parseDisplaysAndGPUs([]byte("not json"))
	assert.Empty(t, gpus)
	assert.Empty(t, displays)
}

func TestParseUSBDevices_Array(t *testing.T) {
	in := []byte(`[
		{"Name":"USB Composite Device","Manufacturer":"(Standard system devices)","PNPDeviceID":"USB\\VID_046D&PID_C52B\\5&abcdef&0&1"},
		{"Name":"USB Mass Storage","Manufacturer":"Compatible USB storage","PNPDeviceID":"USB\\VID_0951&PID_1666\\AA00000000000123"}
	]`)
	devs := parseUSBDevices(in)

	assert.Len(t, devs, 2)
	assert.Equal(t, "USB Composite Device", devs[0].Name)
	assert.Equal(t, "0x046d", devs[0].VendorID)
	assert.Equal(t, "0xc52b", devs[0].ProductID)
	assert.Equal(t, "0x0951", devs[1].VendorID)
	assert.Equal(t, "0x1666", devs[1].ProductID)
}

func TestParseUSBDevices_SingleObjectCollapse(t *testing.T) {
	// ConvertTo-Json can return a bare object when the producing pipeline
	// emits exactly one item — the @(...) wrapper guards against it but we
	// still tolerate the shape.
	in := []byte(`{"Name":"Logitech Webcam","Manufacturer":"Logitech","PNPDeviceID":"USB\\VID_046D&PID_0825\\xyz"}`)
	devs := parseUSBDevices(in)

	assert.Len(t, devs, 1)
	assert.Equal(t, "Logitech Webcam", devs[0].Name)
	assert.Equal(t, "0x046d", devs[0].VendorID)
	assert.Equal(t, "0x0825", devs[0].ProductID)
}

func TestParseUSBDevices_SkipsRowsWithoutVIDPID(t *testing.T) {
	// Some hubs / root devices have PNPDeviceID strings that don't include
	// VID_/PID_ — skip them rather than emitting blank IDs.
	in := []byte(`[
		{"Name":"USB Root Hub","Manufacturer":"(Generic USB Hub)","PNPDeviceID":"USB\\ROOT_HUB30\\4&abc&0"},
		{"Name":"Real device","Manufacturer":"Foo","PNPDeviceID":"USB\\VID_1234&PID_5678\\serial"}
	]`)
	devs := parseUSBDevices(in)

	assert.Len(t, devs, 1)
	assert.Equal(t, "Real device", devs[0].Name)
}

func TestParseUSBDevices_EmptyAndInvalid(t *testing.T) {
	assert.Empty(t, parseUSBDevices([]byte("")))
	assert.Empty(t, parseUSBDevices([]byte("   ")))
	assert.Empty(t, parseUSBDevices([]byte("not json")))
	assert.Empty(t, parseUSBDevices([]byte("[not json]")))
}
