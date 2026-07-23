// chip-detector/pkg/probe/esptool.go
// ESP32/ESP8266 系列芯片串口探测
// 通过发送 sync 帧并解析芯片应答识别型号

package probe

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/tarm/serial"
)

// ESPChipInfo ESP芯片识别结果
type ESPChipInfo struct {
	ChipModel   string `json:"chip_model"`
	MacAddress  string `json:"mac_address,omitempty"`
	FlashSizeMB int    `json:"flash_size_mb,omitempty"`
	Features    string `json:"features,omitempty"`
}

// ESPProbe ESP芯片探测器
type ESPProbe struct {
	timeout time.Duration
}

// NewESPProbe 创建ESP探测器
func NewESPProbe(timeout time.Duration) *ESPProbe {
	if timeout == 0 {
		timeout = 2 * time.Second
	}
	return &ESPProbe{timeout: timeout}
}

// Probe 探测指定串口是否为ESP芯片
func (p *ESPProbe) Probe(portName string, baudRate int) (*ESPChipInfo, error) {
	if baudRate == 0 {
		baudRate = 115200
	}

	cfg := &serial.Config{
		Name:        portName,
		Baud:        baudRate,
		ReadTimeout: p.timeout,
		Size:        8,
		Parity:      serial.ParityNone,
		StopBits:    serial.Stop1,
	}

	port, err := serial.OpenPort(cfg)
	if err != nil {
		return nil, fmt.Errorf("无法打开串口 %s: %w", portName, err)
	}
	defer port.Close()

	// 发送同步帧（ESP协议标准sync序列）
	if err := p.sendSync(port); err != nil {
		return nil, fmt.Errorf("同步失败: %w", err)
	}

	// 读取芯片信息
	chipInfo, err := p.readChipInfo(port)
	if err != nil {
		return nil, fmt.Errorf("读取芯片信息失败: %w", err)
	}

	return chipInfo, nil
}

// sendSync 发送ESP ROM Bootloader同步序列
func (p *ESPProbe) sendSync(port *serial.Port) error {
	// ESP ROM Bootloader 同步协议：
	// 1. 发送 0x07 0x07 0x12 0x20 + 32个 0x55
	// 2. 等待接收 0x01 0x02 表示进入下载模式

	syncFrame := make([]byte, 36)
	syncFrame[0] = 0x07
	syncFrame[1] = 0x07
	syncFrame[2] = 0x12
	syncFrame[3] = 0x20
	for i := 4; i < 36; i++ {
		syncFrame[i] = 0x55
	}

	// 清空缓冲区
	port.Flush()

	// 最多尝试 3 次同步
	for attempt := 0; attempt < 3; attempt++ {
		if _, err := port.Write(syncFrame); err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// 读取响应，期望 0x01 0x02
		resp := make([]byte, 2)
		n, err := port.Read(resp)
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		if n == 2 && resp[0] == 0x01 && resp[1] == 0x02 {
			// 发送确认
			ack := []byte{0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
			port.Write(ack)
			time.Sleep(50 * time.Millisecond)
			return nil
		}

		// 可能是错误响应，重试
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("同步超时，未收到芯片响应（请确认芯片处于下载模式）")
}

// readChipInfo 读取芯片详细信息
func (p *ESPProbe) readChipInfo(port *serial.Port) (*ESPChipInfo, error) {
	// 发送读取芯片信息命令
	// ESP_COMMAND_READ_REG = 0x0A
	cmd := []byte{
		0x00,       // 方向: 请求
		0x0A,       // 命令: READ_REG
		0x04, 0x00, // 数据长度
		0x00, 0x00, 0x00, 0x00, // 校验和(先留空)
	}

	// 读取 CHIP_ID 寄存器地址 0x40001000
	data := make([]byte, 4)
	binary.LittleEndian.PutUint32(data, 0x40001000)
	cmd = append(cmd, data...)

	// 计算校验和
	cmd = p.appendChecksum(cmd)

	if _, err := port.Write(cmd); err != nil {
		return nil, err
	}

	// 读取响应
	resp := make([]byte, 12)
	n, err := port.Read(resp)
	if err != nil || n < 8 {
		return nil, fmt.Errorf("读取响应失败")
	}

	// 解析响应
	// 响应格式: [方向][命令][长度2字节][值4字节][校验和4字节]
	if resp[1] != 0x0A {
		return nil, fmt.Errorf("意外响应命令: 0x%02X", resp[1])
	}

	chipID := binary.LittleEndian.Uint32(resp[4:8])

	info := &ESPChipInfo{}
	info.ChipModel = p.decodeChipModel(chipID)
	info.Features = p.decodeFeatures(chipID)

	return info, nil
}

// decodeChipModel 根据芯片ID解析型号
func (p *ESPProbe) decodeChipModel(chipID uint32) string {
	models := map[uint32]string{
		0x000007c6: "ESP32",
		0x00000005: "ESP32-S3",
		0x00000009: "ESP32-S3(beta2)",
		0x000007c3: "ESP32-C3",
		0x000007c2: "ESP32-C6",
		0x000007c7: "ESP32-H2",
		0x00000001: "ESP32-S2",
		0x000007c1: "ESP32-P4",
		0x00000002: "ESP8266EX",
		0x00000003: "ESP8285",
	}

	if model, ok := models[chipID]; ok {
		return model
	}
	return fmt.Sprintf("Unknown ESP (ID:0x%08X)", chipID)
}

// decodeFeatures 解析芯片特性
func (p *ESPProbe) decodeFeatures(chipID uint32) string {
	features := map[uint32]string{
		0x000007c6: "WiFi+BT, Xtensa LX6",
		0x00000005: "WiFi+BLE5, Xtensa LX7",
		0x000007c3: "WiFi+BLE5, RISC-V",
		0x000007c2: "WiFi6+BLE5, RISC-V",
		0x000007c7: "BLE5/802.15.4, RISC-V",
		0x00000001: "WiFi, Xtensa LX7",
		0x00000002: "WiFi, Xtensa L106",
	}

	if feat, ok := features[chipID]; ok {
		return feat
	}
	return "未知"
}

// appendChecksum 计算并追加ESP协议校验和
func (p *ESPProbe) appendChecksum(frame []byte) []byte {
	// ESP校验和: 从第0字节开始累加到数据结束，结果放在4字节校验和位置
	// 简化实现：跳过校验和位置(索引4-7)计算
	var checksum uint32
	for i := 0; i < len(frame); i++ {
		if i >= 4 && i < 8 {
			continue
		}
		checksum += uint32(frame[i])
	}

	// 写入校验和(如果帧长度足够)
	if len(frame) >= 8 {
		binary.LittleEndian.PutUint32(frame[4:8], checksum)
	}

	return frame
}

// ProbeAutoBaud 自动尝试多个波特率探测
func (p *ESPProbe) ProbeAutoBaud(portName string) (*ESPChipInfo, error) {
	baudRates := []int{115200, 460800, 921600, 230400, 74880}

	for _, baud := range baudRates {
		info, err := p.Probe(portName, baud)
		if err == nil {
			return info, nil
		}
	}

	return nil, fmt.Errorf("所有波特率均无法连接ESP芯片")
}