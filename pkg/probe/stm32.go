// chip-detector/pkg/probe/stm32.go
// STM32 系列芯片 USART Bootloader 握手识别
// 协议: AN3155 (USART protocol used in STM32 bootloader)

package probe

import (
	"fmt"
	"time"

	"github.com/tarm/serial"
)

// STM32ChipInfo STM32芯片识别结果
type STM32ChipInfo struct {
	ChipModel    string `json:"chip_model"`
	BootloaderVer string `json:"bootloader_version,omitempty"`
	PID          uint16 `json:"pid"` // Product ID
}

// STM32Probe STM32探测器
type STM32Probe struct {
	timeout time.Duration
}

// STM32 PID 映射表
var stm32PIDMap = map[uint16]string{
	0x0410: "STM32F40x/41x",
	0x0411: "STM32F401xB/C",
	0x0412: "STM32F401xD/E",
	0x0413: "STM32F407xx/F417xx",
	0x0414: "STM32F411xC/E",
	0x0415: "STM32F415xx",
	0x0416: "STM32F429xx/F439xx",
	0x0417: "STM32F42xxx/43xxx",
	0x0418: "STM32F401xC(256K)",
	0x0419: "STM32F42xxx/43xxx(1M)",
	0x0420: "STM32F446xx",
	0x0421: "STM32F446xx(512K)",
	0x0422: "STM32F469xx/F479xx",
	0x0423: "STM32F469xx(1M)",
	0x0430: "STM32F413xx",
	0x0431: "STM32F423xx",
	0x0440: "STM32L4x1",
	0x0441: "STM32L4x2",
	0x0442: "STM32L4x3",
	0x0444: "STM32L4x5/4x6",
	0x0445: "STM32L4x5/4x6(1M)",
	0x0447: "STM32L4A6",
	0x0448: "STM32L4R/Sxx",
	0x0410: "STM32F40x/41x",
	0x0420: "STM32F446xx",
	0x0430: "STM32F413xx",
	0x0440: "STM32L4x1",
	0x0450: "STM32G0x1",
	0x0451: "STM32G0x4",
	0x0452: "STM32G0x8",
	0x0453: "STM32G0Bx",
	0x0456: "STM32G0Cx",
	0x0460: "STM32G4x1",
	0x0461: "STM32G4x3",
	0x0462: "STM32G4x4",
	0x0463: "STM32G4x5",
	0x0468: "STM32G4x6",
	0x0470: "STM32H72x/73x",
	0x0471: "STM32H74x/75x",
	0x0472: "STM32H7A3/7B3",
	0x0473: "STM32H7B0(128K)",
	0x0480: "STM32WB5x",
	0x0481: "STM32WB3x",
	0x0482: "STM32WB1x",
	0x0490: "STM32WL5x",
	0x0491: "STM32WLEx",
	0x0492: "STM32WL3x",
	0x0412: "STM32F103(中容量)",
	0x0414: "STM32F103(高容量)",
	0x0418: "STM32F105/107",
	0x0420: "STM32F100(中容量)",
	0x0428: "STM32F100(高容量)",
	0x0440: "STM32F030x8",
	0x0441: "STM32F030xC",
	0x0442: "STM32F070xB",
	0x0444: "STM32F030x4/6",
	0x0445: "STM32F070x6",
	0x0448: "STM32F030xC(256K)",
}

// NewSTM32Probe 创建STM32探测器
func NewSTM32Probe(timeout time.Duration) *STM32Probe {
	if timeout == 0 {
		timeout = 2 * time.Second
	}
	return &STM32Probe{timeout: timeout}
}

// Probe 探测指定串口是否为STM32 Bootloader
func (p *STM32Probe) Probe(portName string) (*STM32ChipInfo, error) {
	cfg := &serial.Config{
		Name:        portName,
		Baud:        115200,
		ReadTimeout: p.timeout,
		Size:        8,
		Parity:      serial.ParityEven, // STM32 Bootloader 需要偶校验
		StopBits:    serial.Stop1,
	}

	port, err := serial.OpenPort(cfg)
	if err != nil {
		// 尝试无校验（某些bootloader版本）
		cfg.Parity = serial.ParityNone
		port, err = serial.OpenPort(cfg)
		if err != nil {
			return nil, fmt.Errorf("无法打开串口 %s: %w", portName, err)
		}
	}
	defer port.Close()

	// 清空缓冲区
	port.Flush()

	// 发送 STM32 Bootloader 初始化字节 0x7F
	if _, err := port.Write([]byte{0x7F}); err != nil {
		return nil, fmt.Errorf("发送初始化字节失败: %w", err)
	}

	// 等待 ACK (0x79)
	ack := make([]byte, 1)
	n, err := port.Read(ack)
	if err != nil {
		return nil, fmt.Errorf("未收到STM32 Bootloader响应（请确认芯片处于Bootloader模式）")
	}
	if n != 1 || ack[0] != 0x79 {
		return nil, fmt.Errorf("收到非预期响应: 0x%02X (期望 0x79)", ack[0])
	}

	// 发送 Get Version 命令 (0x00 + 0xFF)
	if _, err := port.Write([]byte{0x00, 0xFF}); err != nil {
		return nil, fmt.Errorf("发送Get Version失败: %w", err)
	}

	// 等待 ACK
	n, err = port.Read(ack)
	if err != nil || n != 1 || ack[0] != 0x79 {
		return nil, fmt.Errorf("Get Version未收到ACK")
	}

	// 读取版本信息（版本号 + 选项字节1 + 选项字节2 + ACK）
	verData := make([]byte, 4)
	n, err = port.Read(verData)
	if err != nil || n < 3 {
		return nil, fmt.Errorf("读取版本信息失败")
	}

	bootloaderVer := fmt.Sprintf("%d.%d", verData[0]>>4, verData[0]&0x0F)

	// 发送 Get ID 命令 (0x02 + 0xFD)
	if _, err := port.Write([]byte{0x02, 0xFD}); err != nil {
		return nil, fmt.Errorf("发送Get ID失败: %w", err)
	}

	// 等待 ACK
	n, err = port.Read(ack)
	if err != nil || n != 1 || ack[0] != 0x79 {
		return nil, fmt.Errorf("Get ID未收到ACK")
	}

	// 读取 PID (2字节)
	pidData := make([]byte, 3) // 长度字节 + PID(2字节)
	n, err = port.Read(pidData)
	if err != nil || n < 3 {
		return nil, fmt.Errorf("读取PID失败")
	}

	pid := uint16(pidData[1])<<8 | uint16(pidData[2])

	// 查找芯片型号
	chipModel := "Unknown STM32"
	if model, ok := stm32PIDMap[pid]; ok {
		chipModel = model
	}

	return &STM32ChipInfo{
		ChipModel:     chipModel,
		BootloaderVer: bootloaderVer,
		PID:           pid,
	}, nil
}