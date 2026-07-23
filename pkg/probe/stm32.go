// chip-detector/pkg/probe/stm32.go
package probe

import (
	"fmt"
	"time"

	"github.com/tarm/serial"
)

type STM32ChipInfo struct {
	ChipModel     string `json:"chip_model"`
	BootloaderVer string `json:"bootloader_version,omitempty"`
	PID           uint16 `json:"pid"`
}

var stm32PIDMap = map[uint16]string{
	0x0410: "STM32F40x/41x",
	0x0411: "STM32F401xB/C",
	0x0412: "STM32F401xD/E / F103(中容量)",
	0x0413: "STM32F407xx/F417xx",
	0x0414: "STM32F411xC/E / F103(高容量)",
	0x0415: "STM32F415xx",
	0x0416: "STM32F429xx/F439xx",
	0x0417: "STM32F42xxx/43xxx",
	0x0418: "STM32F401xC / F105/F107",
	0x0419: "STM32F42xxx/43xxx(1M)",
	0x0420: "STM32F446xx / F100(中容量)",
	0x0421: "STM32F446xx(512K)",
	0x0422: "STM32F469xx/F479xx",
	0x0423: "STM32F469xx(1M)",
	0x0428: "STM32F100(高容量)",
	0x0430: "STM32F413xx",
	0x0431: "STM32F423xx",
	0x0438: "STM32L0xx",
	0x0440: "STM32L4x1 / F030x8",
	0x0441: "STM32L4x2 / F030xC",
	0x0442: "STM32L4x3 / F070xB",
	0x0444: "STM32L4x5/4x6 / F030x4/6",
	0x0445: "STM32L4x5/4x6(1M) / F070x6",
	0x0447: "STM32L4A6",
	0x0448: "STM32L4R/Sxx / F030xC(256K)",
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
	0x0473: "STM32H7B0",
	0x0480: "STM32WB5x",
	0x0481: "STM32WB3x",
	0x0482: "STM32WB1x",
	0x0490: "STM32WL5x",
	0x0491: "STM32WLEx",
	0x0492: "STM32WL3x",
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
	cfg := &serial.Config{
		Name:        portName,
		Baud:        115200,
		ReadTimeout: p.timeout,
		Size:        8,
		Parity:      serial.ParityEven,
		StopBits:    serial.Stop1,
	}

	port, err := serial.OpenPort(cfg)
	if err != nil {
		cfg.Parity = serial.ParityNone
		port, err = serial.OpenPort(cfg)
		if err != nil {
			return nil, fmt.Errorf("无法打开串口 %s: %w", portName, err)
		}
	}
	defer port.Close()

	port.Flush()

	if _, err := port.Write([]byte{0x7F}); err != nil {
		return nil, fmt.Errorf("发送初始化失败: %w", err)
	}

	ack := make([]byte, 1)
	n, err := port.Read(ack)
	if err != nil || n != 1 || ack[0] != 0x79 {
		return nil, fmt.Errorf("未收到ACK (0x79)，请确认芯片处于Bootloader模式")
	}

	if _, err := port.Write([]byte{0x00, 0xFF}); err != nil {
		return nil, err
	}
	n, err = port.Read(ack)
	if err != nil || n != 1 || ack[0] != 0x79 {
		return nil, fmt.Errorf("Get Version失败")
	}

	verData := make([]byte, 4)
	n, err = port.Read(verData)
	if err != nil || n < 3 {
		return nil, fmt.Errorf("读取版本失败")
	}
	bootloaderVer := fmt.Sprintf("%d.%d", verData[0]>>4, verData[0]&0x0F)

	if _, err := port.Write([]byte{0x02, 0xFD}); err != nil {
		return nil, err
	}
	n, err = port.Read(ack)
	if err != nil || n != 1 || ack[0] != 0x79 {
		return nil, fmt.Errorf("Get ID失败")
	}

	pidData := make([]byte, 3)
	n, err = port.Read(pidData)
	if err != nil || n < 3 {
		return nil, fmt.Errorf("读取PID失败")
	}

	pid := uint16(pidData[1])<<8 | uint16(pidData[2])

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