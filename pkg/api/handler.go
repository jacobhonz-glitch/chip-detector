// chip-detector/pkg/api/handler.go
// HTTP REST API + WebSocket 处理器（完整版）
// 集成USB扫描、串口扫描、探测调度器

package api

import (
	"encoding/json"
	"fmt" 
	"log"
	"net/http"
	"sync"
	"time"

	"chip-detector/pkg/cache"
	"chip-detector/pkg/probe"
	"chip-detector/pkg/usb"
)

// APIResponse 统一响应格式
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	Time    int64       `json:"time_ms"`
}

// Handler API处理器
type Handler struct {
	scanner       *usb.Scanner
	serialScanner *usb.SerialScanner
	cache         *cache.Cache
	dispatcher    *probe.Dispatcher
	mu            sync.RWMutex
}

// NewHandler 创建处理器
func NewHandler() (*Handler, error) {
	scanner, err := usb.NewScanner()
	if err != nil {
		return nil, err
	}

	c, err := cache.NewCache()
	if err != nil {
		return nil, err
	}

	return &Handler{
		scanner:       scanner,
		serialScanner: usb.NewSerialScanner(),
		cache:         c,
		dispatcher:    probe.NewDispatcher(10 * time.Second),
	}, nil
}

// writeJSON 写入JSON响应
func writeJSON(w http.ResponseWriter, resp APIResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	resp.Time = time.Now().UnixMilli()
	json.NewEncoder(w).Encode(resp)
}

// ============ API端点 ============

// HandleHealth 健康检查
func (h *Handler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		writeJSON(w, APIResponse{Success: true})
		return
	}
	writeJSON(w, APIResponse{
		Success: true,
		Data: map[string]string{
			"status":  "ok",
			"version": "0.2.0",
			"modules": "usb+serial+probe",
		},
	})
}

// HandleScan USB设备扫描（含串口）
func (h *Handler) HandleScan(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		writeJSON(w, APIResponse{Success: true})
		return
	}

	// 1. USB设备扫描
	usbResult, err := h.scanner.Scan()
	if err != nil {
		writeJSON(w, APIResponse{Success: false, Error: "USB扫描失败: " + err.Error()})
		return
	}

	// 2. 串口扫描
	serialPorts, err := h.serialScanner.Scan()
	if err != nil {
		log.Printf("串口扫描警告: %v", err)
		serialPorts = []usb.SerialPortInfo{}
	}

	// 3. 匹配USB和串口
	matched := h.serialScanner.MatchUSBToSerial(usbResult.Devices, serialPorts)

	writeJSON(w, APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"usb_devices":  usbResult.Devices,
			"serial_ports": matched,
			"total_usb":    len(usbResult.Devices),
			"total_serial": len(matched),
			"duration":     usbResult.Duration,
		},
	})
}

// HandleDetect 单个设备探测
func (h *Handler) HandleDetect(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		writeJSON(w, APIResponse{Success: true})
		return
	}

	var req struct {
		VID          uint16 `json:"vid"`
		PID          uint16 `json:"pid"`
		Serial       string `json:"serial"`
		DeviceClass  string `json:"device_class,omitempty"`
		PortName     string `json:"port_name,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, APIResponse{Success: false, Error: "参数错误: " + err.Error()})
		return
	}

	log.Printf("🔍 探测请求: VID=0x%04X PID=0x%04X Class=%s Port=%s",
		req.VID, req.PID, req.DeviceClass, req.PortName)

	// 1. 查缓存
	if chip, found := h.cache.Lookup(req.VID, req.PID, req.Serial); found {
		log.Printf("✅ 缓存命中: %s", chip)
		writeJSON(w, APIResponse{
			Success: true,
			Data: map[string]interface{}{
				"chip":       chip,
				"from_cache": true,
				"confidence": 0.95,
				"source":     "cache",
			},
		})
		return
	}

	// 2. 调度探测器
	result := h.dispatcher.Dispatch(probe.DispatchRequest{
		DeviceClass:  usb.DeviceClass(req.DeviceClass),
		VID:          req.VID,
		PID:          req.PID,
		SerialNumber: req.Serial,
		PortName:     req.PortName,
	})

	if result.ChipModel == "" || result.Error != "" {
		writeJSON(w, APIResponse{
			Success: false,
			Error:   result.Error,
			Data: map[string]interface{}{
				"source":     result.Source,
				"confidence": result.Confidence,
				"duration":   result.Duration,
			},
		})
		return
	}

	// 3. 写入缓存
	h.cache.Store(req.VID, req.PID, req.Serial, result.ChipModel)
	log.Printf("✅ 探测成功: %s (来源: %s, 置信度: %.0f%%)",
		result.ChipModel, result.Source, result.Confidence*100)

	writeJSON(w, APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"chip":       result.ChipModel,
			"source":     result.Source,
			"confidence": result.Confidence,
			"duration":   result.Duration,
			"details":    result.Details,
		},
	})
}

// HandleQuickDetect 快速全自动识别
func (h *Handler) HandleQuickDetect(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		writeJSON(w, APIResponse{Success: true})
		return
	}

	startTime := time.Now()
	results := make([]map[string]interface{}, 0)

	// 1. USB扫描
	usbResult, err := h.scanner.Scan()
	if err != nil {
		writeJSON(w, APIResponse{Success: false, Error: "USB扫描失败: " + err.Error()})
		return
	}

	// 2. 串口扫描
	serialPorts, _ := h.serialScanner.Scan()
	matched := h.serialScanner.MatchUSBToSerial(usbResult.Devices, serialPorts)

	// 3. 对每个设备尝试识别
	for _, dev := range usbResult.Devices {
		entry := map[string]interface{}{
			"vid":          dev.VID,
			"pid":          dev.PID,
			"serial":       dev.SerialNumber,
			"device_class": dev.DeviceClass,
		}

		// 查找关联的串口
		portName := ""
		for _, sp := range matched {
			if sp.VID == dev.VID && sp.PID == dev.PID {
				portName = sp.PortName
				break
			}
		}
		entry["port_name"] = portName

		// 查缓存
		if chip, found := h.cache.Lookup(dev.VID, dev.PID, dev.SerialNumber); found {
			entry["chip"] = chip
			entry["from_cache"] = true
			entry["confidence"] = 0.95
			entry["source"] = "cache"
			results = append(results, entry)
			continue
		}

		// 探测
		result := h.dispatcher.Dispatch(probe.DispatchRequest{
			DeviceClass:  dev.DeviceClass,
			VID:          dev.VID,
			PID:          dev.PID,
			SerialNumber: dev.SerialNumber,
			PortName:     portName,
		})

		if result.ChipModel != "" && result.Error == "" {
			entry["chip"] = result.ChipModel
			entry["confidence"] = result.Confidence
			entry["source"] = result.Source
			entry["details"] = result.Details
			h.cache.Store(dev.VID, dev.PID, dev.SerialNumber, result.ChipModel)
		} else {
			entry["needs_probe"] = true
			entry["error"] = result.Error
		}

		results = append(results, entry)
	}

	writeJSON(w, APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"devices":       results,
			"total":         len(results),
			"scan_duration": time.Since(startTime).Seconds(),
		},
	})
}

// HandleCacheClear 清除缓存
func (h *Handler) HandleCacheClear(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		writeJSON(w, APIResponse{Success: true})
		return
	}

	if r.Method != "POST" {
		writeJSON(w, APIResponse{Success: false, Error: "仅支持POST请求"})
		return
	}

	// 重新创建缓存实例（清空所有条目）
	newCache, err := cache.NewCache()
	if err != nil {
		writeJSON(w, APIResponse{Success: false, Error: "清除缓存失败: " + err.Error()})
		return
	}
	h.cache = newCache

	log.Println("🗑 缓存已清除")
	writeJSON(w, APIResponse{
		Success: true,
		Data:    map[string]string{"message": "缓存已清除"},
	})
}

// HandleHistory 识别历史
func (h *Handler) HandleHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		writeJSON(w, APIResponse{Success: true})
		return
	}

	// 从缓存中读取所有历史记录
	entries := h.cache.GetAll()

	history := make([]map[string]interface{}, 0, len(entries))
	for _, e := range entries {
		history = append(history, map[string]interface{}{
			"time":         time.UnixMilli(e.LastSeen).Format("2006-01-02 15:04:05"),
			"chip":         e.ChipModel,
			"vid":          fmt.Sprintf("0x%04X", e.VID),
			"pid":          fmt.Sprintf("0x%04X", e.PID),
			"serial":       e.SerialNumber,
			"hit_count":    e.HitCount,
			"method":       "cache", // 可以扩展存储探测方法
		})
	}

	writeJSON(w, APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"history": history,
			"total":   len(history),
		},
	})
}

// ============ 路由 ============

// NewRouter 创建路由
func NewRouter() http.Handler {
	handler, err := NewHandler()
	if err != nil {
		panic(err)
	}

	mux := http.NewServeMux()

	// REST API
	mux.HandleFunc("/api/health", handler.HandleHealth)
	mux.HandleFunc("/api/scan", handler.HandleScan)
	mux.HandleFunc("/api/detect", handler.HandleDetect)
	mux.HandleFunc("/api/quick-detect", handler.HandleQuickDetect)
	mux.HandleFunc("/api/cache/clear", handler.HandleCacheClear)
	mux.HandleFunc("/api/history", handler.HandleHistory)

	// WebSocket
	mux.HandleFunc("/ws", handler.HandleWebSocket)

	// 静态文件
	mux.Handle("/", http.FileServer(http.Dir("./web")))

	return mux
}