// chip-detector/pkg/api/websocket.go
// WebSocket 实时推送 - 设备热插拔通知、探测进度、日志流

package api

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// WSMessage WebSocket消息结构
type WSMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
	Time    int64       `json:"time"`
}

// WSClient WebSocket客户端
type WSClient struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

// WSHub WebSocket连接管理中心
type WSHub struct {
	clients map[*WSClient]bool
	mu      sync.RWMutex
}

var (
	hub      = &WSHub{clients: make(map[*WSClient]bool)}
	hubMutex sync.RWMutex
)

func init() {
	hub = &WSHub{clients: make(map[*WSClient]bool)}
}

// HandleWebSocket 处理WebSocket连接
func (h *Handler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket升级失败: %v", err)
		return
	}

	client := &WSClient{conn: conn}

	hub.mu.Lock()
	hub.clients[client] = true
	hub.mu.Unlock()

	log.Printf("🔌 WebSocket客户端已连接 (当前连接数: %d)", len(hub.clients))

	defer func() {
		hub.mu.Lock()
		delete(hub.clients, client)
		hub.mu.Unlock()
		conn.Close()
		log.Printf("🔌 WebSocket客户端已断开 (当前连接数: %d)", len(hub.clients))
	}()

	// 发送欢迎消息
	client.send(WSMessage{
		Type: "connected",
		Payload: map[string]interface{}{
			"message":    "已连接到Chip Detector",
			"version":    "0.2.0",
			"client_id":  conn.RemoteAddr().String(),
			"cache_stats": h.cache.GetStats(),
		},
		Time: time.Now().UnixMilli(),
	})

	// 消息循环
	for {
		var msg WSMessage
		if err := conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("WebSocket读取错误: %v", err)
			}
			break
		}

		// 处理客户端命令
		h.handleWSCommand(client, msg)
	}
}

// handleWSCommand 处理WebSocket命令
func (h *Handler) handleWSCommand(client *WSClient, msg WSMessage) {
	switch msg.Type {
	case "ping":
		client.send(WSMessage{
			Type:    "pong",
			Payload: map[string]int64{"server_time": time.Now().UnixMilli()},
			Time:    time.Now().UnixMilli(),
		})

	case "scan":
		// 执行完整扫描
		usbResult, err := h.scanner.Scan()
		if err != nil {
			client.send(WSMessage{
				Type:    "error",
				Payload: map[string]string{"message": "USB扫描失败: " + err.Error()},
				Time:    time.Now().UnixMilli(),
			})
			return
		}

		serialPorts, _ := h.serialScanner.Scan()
		matched := h.serialScanner.MatchUSBToSerial(usbResult.Devices, serialPorts)

		client.send(WSMessage{
			Type: "scan_result",
			Payload: map[string]interface{}{
				"usb_devices":  usbResult.Devices,
				"serial_ports": matched,
				"total":        len(usbResult.Devices),
			},
			Time: time.Now().UnixMilli(),
		})

	case "quick_detect":
		// 快速识别
		client.send(WSMessage{
			Type:    "status",
			Payload: map[string]string{"status": "detecting", "message": "正在识别芯片..."},
			Time:    time.Now().UnixMilli(),
		})

		// 执行识别
		usbResult, err := h.scanner.Scan()
		if err != nil {
			client.send(WSMessage{
				Type:    "error",
				Payload: map[string]string{"message": err.Error()},
				Time:    time.Now().UnixMilli(),
			})
			return
		}

		serialPorts, _ := h.serialScanner.Scan()
		matched := h.serialScanner.MatchUSBToSerial(usbResult.Devices, serialPorts)

		results := make([]map[string]interface{}, 0)
		for _, dev := range usbResult.Devices {
			portName := ""
			for _, sp := range matched {
				if sp.VID == dev.VID && sp.PID == dev.PID {
					portName = sp.PortName
					break
				}
			}

			// 查缓存
			if chip, found := h.cache.Lookup(dev.VID, dev.PID, dev.SerialNumber); found {
				results = append(results, map[string]interface{}{
					"vid": dev.VID, "pid": dev.PID,
					"chip": chip, "from_cache": true,
				})
				continue
			}

			// 发送探测进度
			client.send(WSMessage{
				Type: "probe_progress",
				Payload: map[string]interface{}{
					"vid":    dev.VID,
					"pid":    dev.PID,
					"status": "probing",
				},
				Time: time.Now().UnixMilli(),
			})

			// 需要实现探测调度
			results = append(results, map[string]interface{}{
				"vid":          dev.VID,
				"pid":          dev.PID,
				"port_name":    portName,
				"device_class": dev.DeviceClass,
				"needs_probe":  true,
			})
		}

		client.send(WSMessage{
			Type:    "detect_result",
			Payload: results,
			Time:    time.Now().UnixMilli(),
		})

	case "get_history":
		entries := h.cache.GetAll()
		history := make([]map[string]interface{}, 0, len(entries))
		for _, e := range entries {
			history = append(history, map[string]interface{}{
				"time":   time.UnixMilli(e.LastSeen).Format("2006-01-02 15:04:05"),
				"chip":   e.ChipModel,
				"vid":    fmt.Sprintf("0x%04X", e.VID),
				"pid":    fmt.Sprintf("0x%04X", e.PID),
				"source": e.Source,
			})
		}
		client.send(WSMessage{
			Type:    "history",
			Payload: history,
			Time:    time.Now().UnixMilli(),
		})

	case "clear_cache":
		h.cache.Clear()
		client.send(WSMessage{
			Type:    "cache_cleared",
			Payload: map[string]string{"message": "缓存已清除"},
			Time:    time.Now().UnixMilli(),
		})

	default:
		client.send(WSMessage{
			Type:    "error",
			Payload: map[string]string{"message": "未知命令: " + msg.Type},
			Time:    time.Now().UnixMilli(),
		})
	}
}

// send 发送消息给客户端
func (c *WSClient) send(msg WSMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.conn.WriteJSON(msg); err != nil {
		log.Printf("WebSocket发送失败: %v", err)
	}
}

// Broadcast 广播消息给所有客户端
func Broadcast(msg WSMessage) {
	hub.mu.RLock()
	defer hub.mu.RUnlock()

	for client := range hub.clients {
		client.send(msg)
	}
}

// BroadcastUSBEvent 广播USB热插拔事件
func BroadcastUSBEvent(eventType string, device interface{}) {
	Broadcast(WSMessage{
		Type:    eventType,
		Payload: device,
		Time:    time.Now().UnixMilli(),
	})
}

// GetConnectionCount 获取当前连接数
func GetConnectionCount() int {
	hub.mu.RLock()
	defer hub.mu.RUnlock()
	return len(hub.clients)
}