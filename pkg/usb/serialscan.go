// chip-detector/pkg/usb/serialscan.go
// 串口扫描模块

package usb

import (
	"fmt"
	"strings"

	"go.bug.st/serial"
)

// SerialPortInfo 串口信息
type SerialPortInfo struct {
	PortName     string `json:"port_name"`
	Description  string `json:"description"`
	VID          uint16 `json:"vid"`
	PID          uint16 `json:"pid"`
	SerialNumber string `json:"serial_number"`
	IsUSB        bool   `json:"is_usb"`
	Manufacturer string `json:"manufacturer,omitempty"`
}

// SerialScanner 串口扫描器
type SerialScanner struct{}

// NewSerialScanner 创建串口扫描器
func NewSerialScanner() *SerialScanner {
	return &SerialScanner{}
}

// Scan 扫描系统所有串口
func (s *SerialScanner) Scan() ([]SerialPortInfo, error) {
	ports, err := serial.GetPortsList()
	if err != nil {
		return nil, fmt.Errorf("获取串口列表失败: %w", err)
	}

	result := make([]SerialPortInfo, 0, len(ports))
	for _, port := range ports {
		info := SerialPortInfo{
			PortName:    port,
			Description: s.cleanDescription(port),
		}
		result = append(result, info)
	}

	return result, nil
}

// cleanDescription 清理串口描述
func (s *SerialScanner) cleanDescription(port string) string {
	desc := port
	desc = strings.TrimPrefix(desc, "/dev/")
	desc = strings.ReplaceAll(desc, "cu.", "")
	desc = strings.ReplaceAll(desc, "tty.", "")
	return desc
}

// MatchUSBToSerial 将USB设备与串口匹配
func (s *SerialScanner) MatchUSBToSerial(usbDevices []USBDeviceInfo, serialPorts []SerialPortInfo) []SerialPortInfo {
	matched := make([]SerialPortInfo, 0, len(serialPorts))

	for _, sp := range serialPorts {
		for _, usb := range usbDevices {
			if usb.PortName == sp.PortName || s.isMatch(usb, sp) {
				sp.VID = usb.VID
				sp.PID = usb.PID
				sp.SerialNumber = usb.SerialNumber
				sp.IsUSB = true
				sp.Manufacturer = usb.VendorName
				sp.Description = usb.ProductName
				break
			}
		}
		matched = append(matched, sp)
	}

	return matched
}

// isMatch 判断USB设备和串口是否匹配
func (s *SerialScanner) isMatch(usb USBDeviceInfo, sp SerialPortInfo) bool {
	if usb.SerialNumber != "" && strings.Contains(sp.Description, usb.SerialNumber) {
		return true
	}
	if usb.ProductName != "" && strings.Contains(sp.Description, usb.ProductName) {
		return true
	}
	return false
}