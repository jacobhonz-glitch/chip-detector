// chip-detector/pkg/probe/stm32.go
package probe

import (
	"fmt"
	"time"

	"go.bug.st/serial"
)

type STM32ChipInfo struct {
	ChipModel     string `json:"chip_model"`
	BootloaderVer string `json:"bootloader_version,omitempty"`
	PID           uint16 `json:"pid"`
}

var stm32PIDMap = map[uint16]string{
	0x0410: "STM32F40x/41x",
	0x0413: "STM32F407/F417",
	0x0414: "STM32F411/F103(高容量)",
	0x0416: "STM32F429/F439",
	0x0420: "STM32F446/F100",
	0x0440: "STM32L4x1/F030x8",
	0x0441: "STM32L4x2/F030xC",
	0x0444: "STM32L4x5/F030x4",
	0x0450: "STM32G0x1",
	0x0460: "STM32G4x1",
	0x0470: "STM32H72x/73x",
	0x0471: "STM32H74x/75x",
}

type STM32Probe struct {
	timeout time.Duration
}

func NewSTM32Probe(timeout time.Duration) *STM32Probe {
	if timeout == 0 {
		timeout = 2 * time.Second
	}
	return &STM32Probe{timeout: timeout}
}

func (p *STM32Probe) Probe(portName string) (*STM32ChipInfo, error) {
	mode := &serial.Mode{
		BaudRate: 115200,
		DataBits: 8,
		Parity:   serial.EvenParity,
		StopBits: serial.OneStopBit,
	}

	port, err := serial.Open(portName, mode)
	if err != nil {
		mode.Parity = serial.NoParity
		port, err = serial.Open(portName, mode)
		if err != nil {
			return nil, fmt.Errorf("无法打开串口: %w", err)
		}
	}
	defer port.Close()

	port.SetReadTimeout(p.timeout)

	// 清空缓冲
	port.ResetInputBuffer()

	// 发送初始化字节
	if _, err := port.Write([]byte{0x7F}); err != nil {
		return nil, err
	}

	ack := make([]byte, 1)
	if _, err := port.Read(ack); err != nil || ack[0] != 0x79 {
		return nil, fmt.Errorf("非STM32 Bootloader")
	}

	// Get ID
	port.Write([]byte{0x02, 0xFD})
	port.Read(ack)
	if ack[0] != 0x79 {
		return nil, fmt.Errorf("Get ID失败")
	}

	pidData := make([]byte, 3)
	port.Read(pidData)
	pid := uint16(pidData[1])<<8 | uint16(pidData[2])

	model := "Unknown STM32"
	if m, ok := stm32PIDMap[pid]; ok {
		model = m
	}

	return &STM32ChipInfo{
		ChipModel: model,
		PID:       pid,
	}, nil
}