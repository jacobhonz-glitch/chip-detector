// chip-detector/pkg/usb/scanner.go
// USB设备扫描与分类

package usb

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/gousb"
)

// USBDeviceInfo USB设备信息
type USBDeviceInfo struct {
	VID          uint16      `json:"vid"`
	PID          uint16      `json:"pid"`
	VendorName   string      `json:"vendor_name"`
	ProductName  string      `json:"product_name"`
	SerialNumber string      `json:"serial_number"`
	BusNumber    int         `json:"bus_number"`
	PortNumber   int         `json:"port_number"`
	DeviceClass  DeviceClass `json:"device_class"`
	PortName     string      `json:"port_name,omitempty"`
}

// DeviceClass 设备类型
type DeviceClass string

const (
	ClassDebugger   DeviceClass = "debugger"
	ClassSerial     DeviceClass = "serial"
	ClassBootloader DeviceClass = "bootloader"
	ClassUnknown    DeviceClass = "unknown"
)

// ScanResult 扫描结果
type ScanResult struct {
	Devices   []USBDeviceInfo `json:"devices"`
	Timestamp int64           `json:"timestamp"`
	Duration  float64         `json:"duration_seconds"`
}

// Scanner USB扫描器
type Scanner struct {
	ctx           *gousb.Context
	debuggerDB    map[uint16]map[uint16]string
	serialDB      map[uint16]map[uint16]string
	bootloaderDB  map[uint16]map[uint16]string
	mu            sync.RWMutex
}

// NewScanner 创建扫描器
func NewScanner() (*Scanner, error) {
	ctx := gousb.NewContext()

	s := &Scanner{
		ctx:          ctx,
		debuggerDB:   make(map[uint16]map[uint16]string),
		serialDB:     make(map[uint16]map[uint16]string),
		bootloaderDB: make(map[uint16]map[uint16]string),
	}

	s.initDatabases()
	return s, nil
}

// initDatabases 初始化已知设备数据库
func (s *Scanner) initDatabases() {
	// 调试器
	s.debuggerDB[0x0483] = map[uint16]string{
		0x3748: "ST-Link/V2",
		0x374b: "ST-Link/V2.1",
		0x374f: "ST-Link/V3",
		0x3753: "ST-Link/V3E",
	}
	s.debuggerDB[0x0d28] = map[uint16]string{
		0x0204: "DAPLink",
	}
	s.debuggerDB[0x1366] = map[uint16]string{
		0x0101: "J-Link",
		0x0105: "J-Link EDU",
	}
	s.debuggerDB[0x1a86] = map[uint16]string{
		0x8010: "WCH-Link",
		0x8011: "WCH-LinkE",
	}

	// USB转串口
	s.serialDB[0x1a86] = map[uint16]string{
		0x7523: "CH340",
		0x55d4: "CH9102",
	}
	s.serialDB[0x10c4] = map[uint16]string{
		0xea60: "CP2102",
		0xea70: "CP2105",
	}
	s.serialDB[0x0403] = map[uint16]string{
		0x6001: "FT232",
		0x6015: "FT231X",
	}

	// Bootloader
	s.bootloaderDB[0x2e8a] = map[uint16]string{
		0x0003: "RP2040 BOOTSEL",
		0x0005: "RP2350 BOOTSEL",
	}
	s.bootloaderDB[0x0483] = map[uint16]string{
		0xdf11: "STM32 DFU",
	}
}

// Scan 执行完整扫描
func (s *Scanner) Scan() (*ScanResult, error) {
	startTime := time.Now()
	result := &ScanResult{
		Devices: make([]USBDeviceInfo, 0),
	}

	devices, err := s.ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		return true
	})
	if err != nil {
		return nil, fmt.Errorf("USB枚举失败: %w", err)
	}
	defer func() {
		for _, d := range devices {
			d.Close()
		}
	}()

	for _, dev := range devices {
		desc := dev.Desc
		info := USBDeviceInfo{
			VID:        uint16(desc.Vendor),
			PID:        uint16(desc.Product),
			BusNumber:  dev.Bus,
			PortNumber: dev.Port,
		}

		if vendorStr, err := dev.Vendor(); err == nil {
			info.VendorName = vendorStr
		}
		if productStr, err := dev.Product(); err == nil {
			info.ProductName = productStr
		}
		if serialStr, err := dev.SerialNumber(); err == nil {
			info.SerialNumber = serialStr
		}

		info.DeviceClass = s.classify(info.VID, info.PID)
		result.Devices = append(result.Devices, info)
	}

	result.Duration = time.Since(startTime).Seconds()
	result.Timestamp = time.Now().UnixMilli()

	return result, nil
}

// classify 对设备进行分类
func (s *Scanner) classify(vid, pid uint16) DeviceClass {
	if pids, ok := s.debuggerDB[vid]; ok {
		if _, ok := pids[pid]; ok {
			return ClassDebugger
		}
	}
	if pids, ok := s.bootloaderDB[vid]; ok {
		if _, ok := pids[pid]; ok {
			return ClassBootloader
		}
	}
	if pids, ok := s.serialDB[vid]; ok {
		if _, ok := pids[pid]; ok {
			return ClassSerial
		}
	}
	return ClassUnknown
}

// Close 关闭扫描器
func (s *Scanner) Close() error {
	return s.ctx.Close()
}