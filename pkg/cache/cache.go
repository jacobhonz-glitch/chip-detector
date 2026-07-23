// chip-detector/pkg/cache/cache.go
// 本地缓存模块 - 存储识别结果，加速重复识别

package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CacheEntry 缓存条目
type CacheEntry struct {
	VID          uint16 `json:"vid"`
	PID          uint16 `json:"pid"`
	SerialNumber string `json:"serial_number"`
	ChipModel    string `json:"chip_model"`
	Source       string `json:"source"`        // 探测来源: swd/esptool/stm32-bootloader/cache
	Confidence   float64 `json:"confidence"`
	FirstSeen    int64  `json:"first_seen"`
	LastSeen     int64  `json:"last_seen"`
	HitCount     int    `json:"hit_count"`
}

// Cache 本地缓存
type Cache struct {
	entries map[string]*CacheEntry
	path    string
	mu      sync.RWMutex
}

// NewCache 创建缓存实例
func NewCache() (*Cache, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}

	cachePath := filepath.Join(cacheDir, "chip-detector", "cache.json")
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		return nil, fmt.Errorf("创建缓存目录失败: %w", err)
	}

	c := &Cache{
		entries: make(map[string]*CacheEntry),
		path:    cachePath,
	}

	c.load()
	return c, nil
}

// Lookup 查找缓存
func (c *Cache) Lookup(vid, pid uint16, serialNumber string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := c.makeKey(vid, pid, serialNumber)
	if entry, ok := c.entries[key]; ok {
		entry.LastSeen = time.Now().UnixMilli()
		entry.HitCount++
		c.save()
		return entry.ChipModel, true
	}

	// 如果没有序列号，尝试只用VID/PID匹配
	if serialNumber == "" {
		for _, entry := range c.entries {  // 改这里：k 改成 _
			if entry.VID == vid && entry.PID == pid {
				entry.LastSeen = time.Now().UnixMilli()
				entry.HitCount++
				return entry.ChipModel, true
			}
		}
	}

	return "", false
}

// Store 存储到缓存
func (c *Cache) Store(vid, pid uint16, serialNumber, chipModel string) {
	c.StoreWithDetails(vid, pid, serialNumber, chipModel, "unknown", 0.9)
}

// StoreWithDetails 存储带详细信息的缓存
func (c *Cache) StoreWithDetails(vid, pid uint16, serialNumber, chipModel, source string, confidence float64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := c.makeKey(vid, pid, serialNumber)
	now := time.Now().UnixMilli()

	if entry, ok := c.entries[key]; ok {
		entry.ChipModel = chipModel
		entry.Source = source
		entry.Confidence = confidence
		entry.LastSeen = now
		entry.HitCount++
	} else {
		c.entries[key] = &CacheEntry{
			VID:          vid,
			PID:          pid,
			SerialNumber: serialNumber,
			ChipModel:    chipModel,
			Source:       source,
			Confidence:   confidence,
			FirstSeen:    now,
			LastSeen:     now,
			HitCount:     1,
		}
	}

	c.save()
}

// GetAll 获取所有缓存条目
func (c *Cache) GetAll() []CacheEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entries := make([]CacheEntry, 0, len(c.entries))
	for _, e := range c.entries {
		entries = append(entries, *e)
	}
	return entries
}

// GetStats 获取缓存统计
func (c *Cache) GetStats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := len(c.entries)
	hitSum := 0
	for _, e := range c.entries {
		hitSum += e.HitCount
	}

	return map[string]interface{}{
		"total_entries":  total,
		"total_hits":     hitSum,
		"cache_file":     c.path,
		"cache_size_kb":  c.fileSize(),
	}
}

// Clear 清除所有缓存
func (c *Cache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*CacheEntry)
	return os.WriteFile(c.path, []byte("{}"), 0644)
}

// DeleteEntry 删除单条缓存
func (c *Cache) DeleteEntry(vid, pid uint16, serialNumber string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := c.makeKey(vid, pid, serialNumber)
	if _, ok := c.entries[key]; ok {
		delete(c.entries, key)
		c.save()
		return true
	}
	return false
}

// makeKey 生成缓存键
func (c *Cache) makeKey(vid, pid uint16, serialNumber string) string {
	return fmt.Sprintf("%04x:%04x:%s", vid, pid, serialNumber)
}

// load 从磁盘加载
func (c *Cache) load() {
	data, err := os.ReadFile(c.path)
	if err != nil {
		return // 文件不存在是正常的
	}
	json.Unmarshal(data, &c.entries)
}

// save 保存到磁盘
func (c *Cache) save() {
	data, err := json.MarshalIndent(c.entries, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(c.path, data, 0644)
}

// fileSize 获取缓存文件大小
func (c *Cache) fileSize() float64 {
	info, err := os.Stat(c.path)
	if err != nil {
		return 0
	}
	return float64(info.Size()) / 1024.0
}