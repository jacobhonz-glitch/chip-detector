// chip-detector/pkg/probe/swd.go
// SWD (Serial Wire Debug) 探测器
// 通过调试器读取目标芯片的 IDCODE 寄存器来识别 ARM Cortex-M 系列芯片
// 支持: ST-Link, DAP-Link, J-Link (通过 OpenOCD 或 pyOCD 协议)

package probe

import (
	"encoding/binary"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// SWDChipInfo SWD探测结果
type SWDChipInfo struct {
	ChipModel   string `json:"chip_model"`
	CoreType    string `json:"core_type"`
	IDCODE      uint32 `json:"idcode"`
	DebuggerType string `json:"debugger_type"`
}

// ARM IDCODE 解码表
var armIDCODEDecoder = map[uint32]string{
	// STM32 系列
	0x1BA01477: "STM32F1 (Cortex-M3)",
	0x2BA01477: "STM32F4 (Cortex-M4)",
	0x4BA00477: "STM32F7 (Cortex-M7)",
	0x6BA02477: "STM32H7 (Cortex-M7)",
	0x0BC11477: "STM32L0 (Cortex-M0+)",
	0x0BA01477: "STM32L1 (Cortex-M3)",
	0x2BA02477: "STM32L4 (Cortex-M4)",
	0x4BA02477: "STM32L5 (Cortex-M33)",
	0x3BA02477: "STM32G0 (Cortex-M0+)",
	0x4BA02477: "STM32G4 (Cortex-M4)",
	0x6BA02477: "STM32WB (Cortex-M4+M0+)",
	0x0BA03477: "STM32U5 (Cortex-M33)",

	// Nordic
	0x2BA01477: "nRF51 (Cortex-M0)",
	0x4BA00477: "nRF52 (Cortex-M4)",
	0x6BA02477: "nRF53 (Cortex-M33)",

	// NXP
	0x0BC11477: "LPC800 (Cortex-M0+)",
	0x2BA01477: "LPC1700 (Cortex-M3)",
	0x4BA00477: "LPC4300 (Cortex-M4)",

	// Atmel/Microchip
	0x0BC11477: "SAMD21 (Cortex-M0+)",
	0x2BA01477: "SAMD51 (Cortex-M4)",

	// GD32
	0x1BA01477: "GD32F1 (Cortex-M3)",
	0x2BA01477: "GD32F4 (Cortex-M4)",
}

// coreTypeDecoder 根据IDCODE位段判断核心类型
func decodeCoreType(idcode uint32) string {
	// ARM IDCODE 格式: [31:28]版本 [27:12]部件号 [11:1]JEDEC [0]1
	partNum := (idcode >> 12) & 0xFFFF

	parts := map[uint32]string{
		0xC20: "Cortex-M0",
		0xC21: "Cortex-M1",
		0xC23: "Cortex-M3",
		0xC24: "Cortex-M4",
		0xC27: "Cortex-M7",
		0xC60: "Cortex-M0+",
		0xD20: "Cortex-M23",
		0xD21: "Cortex-M33",
		0xD22: "Cortex-M55",
		0xD23: "Cortex-M85",
	}

	if core, ok := parts[partNum]; ok {
		return core
	}
	return fmt.Sprintf("ARM Core (Part:0x%04X)", partNum)
}

// SWDProbe SWD探测器
type SWDProbe struct {
	openocdPath string
	timeout     time.Duration
}

// NewSWDProbe 创建SWD探测器
func NewSWDProbe(timeout time.Duration) *SWDProbe {
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	// 查找 OpenOCD 路径
	openocdPath := "openocd"
	if path, err := exec.LookPath("openocd"); err == nil {
		openocdPath = path
	}

	return &SWDProbe{
		openocdPath: openocdPath,
		timeout:     timeout,
	}
}

// Probe 使用OpenOCD通过SWD读取芯片IDCODE
func (p *SWDProbe) Probe(debuggerType string, debuggerSerial string) (*SWDChipInfo, error) {
	// 根据调试器类型选择OpenOCD配置文件
	interfaceCfg := p.getInterfaceConfig(debuggerType)
	if interfaceCfg == "" {
		return nil, fmt.Errorf("不支持的调试器类型: %s (支持: stlink, cmsis-dap, jlink)", debuggerType)
	}

	// 构建 OpenOCD 命令
	// openocd -f interface/stlink.cfg -c "init; dap info 0; exit"
	args := []string{
		"-f", interfaceCfg,
		"-c", "init",
		"-c", "dap info 0",
		"-c", "exit",
	}

	// 如果有调试器序列号，添加选择参数
	if debuggerSerial != "" {
		selectArg := fmt.Sprintf("hla_serial %s", debuggerSerial)
		args = append([]string{"-c", selectArg}, args...)
	}

	cmd := exec.Command(p.openocdPath, args...)

	// 执行命令并捕获输出
	output, err := cmd.CombinedOutput()
	if err != nil {
		// OpenOCD 即使成功也可能返回非零退出码
		// 检查输出中是否包含有效信息
		if !strings.Contains(string(output), "IDCODE") {
			return nil, fmt.Errorf("OpenOCD执行失败: %w\n输出: %s", err, string(output))
		}
	}

	// 解析输出中的 IDCODE
	idcode, err := p.parseIDCODE(string(output))
	if err != nil {
		return nil, fmt.Errorf("解析IDCODE失败: %w", err)
	}

	info := &SWDChipInfo{
		IDCODE:       idcode,
		CoreType:     decodeCoreType(idcode),
		DebuggerType: debuggerType,
	}

	// 尝试匹配已知芯片型号
	if model, ok := armIDCODEDecoder[idcode]; ok {
		info.ChipModel = model
	} else {
		// 根据核心类型和IDCODE给出近似型号
		info.ChipModel = fmt.Sprintf("%s (IDCODE: 0x%08X)", info.CoreType, idcode)
	}

	return info, nil
}

// getInterfaceConfig 获取OpenOCD接口配置
func (p *SWDProbe) getInterfaceConfig(debuggerType string) string {
	configs := map[string]string{
		"stlink":    "interface/stlink.cfg",
		"cmsis-dap": "interface/cmsis-dap.cfg",
		"dap-link":  "interface/cmsis-dap.cfg",
		"dap":       "interface/cmsis-dap.cfg",
		"jlink":     "interface/jlink.cfg",
	}

	return configs[strings.ToLower(debuggerType)]
}

// parseIDCODE 从OpenOCD输出中解析IDCODE
func (p *SWDProbe) parseIDCODE(output string) (uint32, error) {
	// 匹配格式: IDCODE 0x1BA01477
	re := regexp.MustCompile(`IDCODE\s+(0x[0-9a-fA-F]+)`)
	matches := re.FindStringSubmatch(output)
	if len(matches) < 2 {
		// 尝试另一种格式: TAP: STM32F407.bs (0x2ba01477)
		re2 := regexp.MustCompile(`TAP:.*\((0x[0-9a-fA-F]+)\)`)
		matches2 := re2.FindStringSubmatch(output)
		if len(matches2) < 2 {
			return 0, fmt.Errorf("输出中未找到IDCODE")
		}
		matches = matches2
	}

	// 解析十六进制
	idcode, err := strconv.ParseUint(strings.TrimPrefix(matches[1], "0x"), 16, 32)
	if err != nil {
		return 0, fmt.Errorf("IDCODE解析失败: %w", err)
	}

	return uint32(idcode), nil
}

// ProbeWithPyOCD 使用 pyOCD 作为备选方案探测
func (p *SWDProbe) ProbeWithPyOCD() (*SWDChipInfo, error) {
	// pyocd list --targets 可以列出已连接的目标
	cmd := exec.Command("pyocd", "list", "--targets")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("pyOCD执行失败: %w", err)
	}

	// pyOCD 输出格式: Unique ID    Target    Board
	//                  123456789   stm32f407vet6   ...
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Unique") || strings.HasPrefix(line, "---") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) >= 2 {
			return &SWDChipInfo{
				ChipModel:    fields[1],
				CoreType:     "Cortex-M",
				DebuggerType: "auto(pyocd)",
			}, nil
		}
	}

	return nil, fmt.Errorf("pyOCD未找到目标芯片")
}

// readRegister 通过SWD读取寄存器（简化实现，依赖外部工具）
func (p *SWDProbe) readRegister(debuggerType string, regAddr uint32) (uint32, error) {
	// 这里可以扩展实现原生SWD协议
	// 当前版本依赖 OpenOCD/pyOCD 封装
	return 0, fmt.Errorf("原生SWD读取未实现，请使用OpenOCD或pyOCD")
}

// 辅助函数：避免未使用的导入警告
var _ = binary.LittleEndian