// chip-detector/pkg/probe/dispatcher.go
// 探测调度器 - 根据设备类型并行调度合适的探测器

package probe

import (
	"fmt"
	"sync"
	"time"

	"chip-detector/pkg/usb"
)

// ProbeResult 探测结果
type ProbeResult struct {
	ChipModel  string      `json:"chip_model"`
	Source     string      `json:"source"`
	Confidence float64     `json:"confidence"`
	Duration   float64     `json:"duration_seconds"`
	Details    interface{} `json:"details,omitempty"`
	Error      string      `json:"error,omitempty"`
}

// DispatchRequest 探测请求
type DispatchRequest struct {
	DeviceClass  usb.DeviceClass `json:"device_class"`
	VID          uint16          `json:"vid"`
	PID          uint16          `json:"pid"`
	SerialNumber string          `json:"serial_number"`
	PortName     string          `json:"port_name,omitempty"`
	Timeout      time.Duration   `json:"-"`
}

// Dispatcher 探测调度器
type Dispatcher struct {
	timeout     time.Duration
	esptool     *ESPProbe
	stm32       *STM32Probe
	swd         *SWDProbe
}

// NewDispatcher 创建探测调度器
func NewDispatcher(timeout time.Duration) *Dispatcher {
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	return &Dispatcher{
		timeout: timeout,
		esptool: NewESPProbe(timeout),
		stm32:   NewSTM32Probe(timeout),
		swd:     NewSWDProbe(timeout),
	}
}

// Dispatch 根据设备类型调度合适的探测器（并行）
func (d *Dispatcher) Dispatch(req DispatchRequest) *ProbeResult {
	// 获取该设备类型对应的探测器列表
	probes := d.selectProbes(req)

	if len(probes) == 0 {
		return &ProbeResult{
			Source:     "none",
			Confidence: 0,
			Error:      fmt.Sprintf("无可用探测器 (设备类型: %s)", req.DeviceClass),
		}
	}

	// 并行执行所有探测器
	resultChan := make(chan ProbeResult, len(probes))
	var wg sync.WaitGroup
	startTime := time.Now()

	for _, probeFunc := range probes {
		wg.Add(1)
		go func(pf probeFunc) {
			defer wg.Done()
			result := pf(req)
			resultChan <- result
		}(probeFunc)
	}

	// 等待所有探测器完成或超时
	doneChan := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneChan)
	}()

	select {
	case <-doneChan:
		// 所有探测器完成
	case <-time.After(d.timeout):
		// 超时
		return &ProbeResult{
			Source:     "timeout",
			Confidence: 0,
			Duration:   d.timeout.Seconds(),
			Error:      fmt.Sprintf("探测超时 (%v)", d.timeout),
		}
	}

	close(resultChan)

	// 收集结果，返回置信度最高的
	var bestResult *ProbeResult
	for result := range resultChan {
		if result.ChipModel != "" && result.Error == "" {
			if bestResult == nil || result.Confidence > bestResult.Confidence {
				r := result // 避免循环变量问题
				bestResult = &r
			}
		}
	}

	if bestResult != nil {
		bestResult.Duration = time.Since(startTime).Seconds()
		return bestResult
	}

	return &ProbeResult{
		Source:     "all_failed",
		Confidence: 0,
		Duration:   time.Since(startTime).Seconds(),
		Error:      "所有探测器均未识别出芯片",
	}
}

// probeFunc 探测器函数类型
type probeFunc func(req DispatchRequest) ProbeResult

// selectProbes 根据设备类型选择合适的探测器
func (d *Dispatcher) selectProbes(req DispatchRequest) []probeFunc {
	var probes []probeFunc

	switch req.DeviceClass {
	case usb.ClassDebugger:
		// 调试器 → SWD探测
		probes = append(probes, d.probeSWD)

	case usb.ClassSerial, usb.ClassUnknown:
		// 串口设备 → ESP探测 + STM32握手
		if req.PortName != "" {
			probes = append(probes, d.probeESP)
			probes = append(probes, d.probeSTM32)
		}

	case usb.ClassBootloader:
		// Bootloader → 根据VID/PID直接匹配或ESP探测
		if d.isESPVID(req.VID) {
			probes = append(probes, d.probeESP)
		} else {
			probes = append(probes, d.probeBootloader)
		}
	}

	// 如果分类失败但有串口，仍然尝试串口探测
	if len(probes) == 0 && req.PortName != "" {
		probes = append(probes, d.probeESP)
		probes = append(probes, d.probeSTM32)
	}

	return probes
}

// probeSWD SWD探测
func (d *Dispatcher) probeSWD(req DispatchRequest) ProbeResult {
	var debuggerType string
	switch {
	case req.VID == 0x0483 && (req.PID == 0x3748 || req.PID == 0x374B || req.PID == 0x374F):
		debuggerType = "stlink"
	case req.VID == 0x0D28 && req.PID == 0x0204:
		debuggerType = "cmsis-dap"
	case req.VID == 0x1366:
		debuggerType = "jlink"
	case req.VID == 0x1A86 && (req.PID == 0x8010 || req.PID == 0x8011):
		debuggerType = "cmsis-dap"
	default:
		debuggerType = "cmsis-dap"
	}

	info, err := d.swd.Probe(debuggerType, req.SerialNumber)
	if err != nil {
		// 尝试 pyOCD 备选
		info2, err2 := d.swd.ProbeWithPyOCD()
		if err2 != nil {
			return ProbeResult{
				Source:     "swd",
				Confidence: 0,
				Error:      fmt.Sprintf("SWD: %s / pyOCD: %s", err.Error(), err2.Error()),
			}
		}
		info = info2
	}

	return ProbeResult{
		ChipModel:  info.ChipModel,
		Source:     "swd",
		Confidence: 0.99,
		Details:    info,
	}
}

// probeESP ESP芯片探测
func (d *Dispatcher) probeESP(req DispatchRequest) ProbeResult {
	if req.PortName == "" {
		return ProbeResult{
			Source:     "esptool",
			Confidence: 0,
			Error:      "无串口号，无法进行ESP探测",
		}
	}

	info, err := d.esptool.ProbeAutoBaud(req.PortName)
	if err != nil {
		return ProbeResult{
			Source:     "esptool",
			Confidence: 0,
			Error:      err.Error(),
		}
	}

	return ProbeResult{
		ChipModel:  info.ChipModel,
		Source:     "esptool",
		Confidence: 0.98,
		Details:    info,
	}
}

// probeSTM32 STM32 Bootloader探测
func (d *Dispatcher) probeSTM32(req DispatchRequest) ProbeResult {
	if req.PortName == "" {
		return ProbeResult{
			Source:     "stm32-bootloader",
			Confidence: 0,
			Error:      "无串口号，无法进行STM32探测",
		}
	}

	info, err := d.stm32.Probe(req.PortName)
	if err != nil {
		return ProbeResult{
			Source:     "stm32-bootloader",
			Confidence: 0,
			Error:      err.Error(),
		}
	}

	return ProbeResult{
		ChipModel:  info.ChipModel,
		Source:     "stm32-bootloader",
		Confidence: 0.95,
		Details:    info,
	}
}

// probeBootloader 已知Bootloader设备直接匹配
func (d *Dispatcher) probeBootloader(req DispatchRequest) ProbeResult {
	bootloaderMap := map[uint16]map[uint16]string{
		0x2E8A: {0x0003: "RP2040", 0x0005: "RP2350"},
		0x0483: {0xDF11: "STM32 (DFU模式)"},
		0x303A: {0x1001: "ESP32-S3 (原生USB)", 0x1002: "ESP32-C3 (原生USB)"},
		0x1A86: {0x7523: "CH340 (可能后接任何MCU)", 0x8010: "WCH-Link"},
	}

	if pids, ok := bootloaderMap[req.VID]; ok {
		if model, ok := pids[req.PID]; ok {
			return ProbeResult{
				ChipModel:  model,
				Source:     "bootloader-id",
				Confidence: 0.85,
				Details: map[string]interface{}{
					"vid":     req.VID,
					"pid":     req.PID,
					"matched": true,
				},
			}
		}
	}

	return ProbeResult{
		Source:     "bootloader-id",
		Confidence: 0,
		Error:      fmt.Sprintf("未知Bootloader: VID=0x%04X PID=0x%04X", req.VID, req.PID),
	}
}

// isESPVID 判断VID是否为乐鑫
func (d *Dispatcher) isESPVID(vid uint16) bool {
	return vid == 0x303A || vid == 0x10C4 || vid == 0x1A86
}