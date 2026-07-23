// chip-detector/pkg/usb/scanner.go
package usb

import (
	// // "fmt"
	"strings"
	"time"

	"go.bug.st/serial"
)

type USBDeviceInfo struct {
	VID          uint16      `json:"vid"`
	PID          uint16      `json:"pid"`
	VendorName   string      `json:"vendor_name"`
	ProductName  string      `json:"product_name"`
	SerialNumber string      `json:"serial_number"`
	PortName     string      `json:"port_name"`
	DeviceClass  DeviceClass `json:"device_class"`
}

type DeviceClass string

const (
	ClassDebugger   DeviceClass = "debugger"
	ClassSerial     DeviceClass = "serial"
	ClassBootloader DeviceClass = "bootloader"
	ClassUnknown    DeviceClass = "unknown"
)

type ScanResult struct {
	Devices   []USBDeviceInfo `json:"devices"`
	Timestamp int64           `json:"timestamp"`
	Duration  float64         `json:"duration_seconds"`
}

type Scanner struct {
	serialDB     map[string]string // portName -> chip hint
}

func NewScanner() (*Scanner, error) {
	return &Scanner{
		serialDB: make(map[string]string),
	}, nil
}

func (s *Scanner) Scan() (*ScanResult, error) {
	startTime := time.Now()
	result := &ScanResult{Devices: make([]USBDeviceInfo, 0)}

	ports, err := serial.GetPortsList()
	if err != nil {
		return result, nil // 返回空列表不报错
	}

	for _, port := range ports {
		info := USBDeviceInfo{
			PortName:    port,
			DeviceClass: ClassSerial,
			ProductName: port,
		}
		result.Devices = append(result.Devices, info)
	}

	// 尝试匹配已知开发板
	for i := range result.Devices {
		port := result.Devices[i].PortName
		lower := strings.ToLower(port)

		if strings.Contains(lower, "usb") || strings.Contains(lower, "tty") {
			result.Devices[i].DeviceClass = ClassSerial
		}
		if strings.Contains(lower, "cu.") || strings.Contains(lower, "com") {
			result.Devices[i].DeviceClass = ClassSerial
		}
	}

	result.Duration = time.Since(startTime).Seconds()
	result.Timestamp = time.Now().UnixMilli()
	return result, nil
}

func (s *Scanner) classify(port string) DeviceClass {
	lower := strings.ToLower(port)
	if strings.Contains(lower, "stlink") || strings.Contains(lower, "debug") {
		return ClassDebugger
	}
	return ClassSerial
}

func (s *Scanner) Close() error {
	return nil
}

// 为兼容性保留
func decodeCoreType(id uint32) string { return "Unknown" }