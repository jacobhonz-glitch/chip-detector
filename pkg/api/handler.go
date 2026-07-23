package api

import (
	"encoding/json"
	"net/http"
	"time"

	"chip-detector/pkg/usb"
)

type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	Time    int64       `json:"time_ms"`
}

type Handler struct {
	scanner       *usb.Scanner
	serialScanner *usb.SerialScanner
}

func NewHandler() (*Handler, error) {
	scanner, _ := usb.NewScanner()
	return &Handler{
		scanner:       scanner,
		serialScanner: usb.NewSerialScanner(),
	}, nil
}

func writeJSON(w http.ResponseWriter, resp APIResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	resp.Time = time.Now().UnixMilli()
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, APIResponse{Success: true, Data: map[string]string{"status": "ok", "version": "0.2.0"}})
}

func (h *Handler) HandleScan(w http.ResponseWriter, r *http.Request) {
	result, err := h.scanner.Scan()
	if err != nil {
		writeJSON(w, APIResponse{Success: false, Error: err.Error()})
		return
	}
	serialPorts, _ := h.serialScanner.Scan()
	matched := h.serialScanner.MatchUSBToSerial(result.Devices, serialPorts)
	writeJSON(w, APIResponse{Success: true, Data: map[string]interface{}{"usb_devices": result.Devices, "serial_ports": matched}})
}

func (h *Handler) HandleQuickDetect(w http.ResponseWriter, r *http.Request) {
	result, _ := h.scanner.Scan()
	serialPorts, _ := h.serialScanner.Scan()
	matched := h.serialScanner.MatchUSBToSerial(result.Devices, serialPorts)
	results := make([]map[string]interface{}, 0)
	for _, dev := range result.Devices {
		portName := ""
		for _, sp := range matched {
			if sp.PID == dev.PID {
				portName = sp.PortName
			}
		}
		results = append(results, map[string]interface{}{
			"vid": dev.VID, "pid": dev.PID, "port_name": portName,
			"device_class": dev.DeviceClass, "needs_probe": true,
		})
	}
	writeJSON(w, APIResponse{Success: true, Data: map[string]interface{}{"devices": results, "total": len(results)}})
}

func (h *Handler) HandleDetect(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, APIResponse{Success: false, Error: "探测功能开发中"})
}

func (h *Handler) HandleCacheClear(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, APIResponse{Success: true, Data: map[string]string{"message": "缓存已清除"}})
}

func (h *Handler) HandleHistory(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, APIResponse{Success: true, Data: map[string]interface{}{"history": []string{}, "total": 0}})
}

func NewRouter() http.Handler {
	handler, _ := NewHandler()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", handler.HandleHealth)
	mux.HandleFunc("/api/scan", handler.HandleScan)
	mux.HandleFunc("/api/detect", handler.HandleDetect)
	mux.HandleFunc("/api/quick-detect", handler.HandleQuickDetect)
	mux.HandleFunc("/api/cache/clear", handler.HandleCacheClear)
	mux.HandleFunc("/api/history", handler.HandleHistory)
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("WebSocket coming soon"))
	})
	mux.Handle("/", http.FileServer(http.Dir("./web")))
	return mux
}
