// Package config 负责配置加载与首启落盘。
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// Config 是运行期配置。密钥只存本地文件，接口返回时脱敏。
type Config struct {
	APIKey      string `json:"api_key"`
	Proxy       string `json:"proxy"`
	Domain      string `json:"domain"`
	DefaultJobs int    `json:"default_jobs"`
	DefaultWait int    `json:"default_wait"`
	AddPassword bool   `json:"add_password"`
	AddMFA      bool   `json:"add_mfa"`

	path string
	mu   sync.RWMutex
}

// Default 返回首启默认值。
func Default() *Config {
	return &Config{
		Domain:      "icloud.com",
		DefaultJobs: 1,
		DefaultWait: 5,
		AddPassword: true,
		AddMFA:      true,
	}
}

// Load 读取配置，不存在则写默认值（600 权限）。
func Load(path string) (*Config, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, err
	}
	c := Default()
	c.path = path
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, c.save()
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, c); err != nil {
		return nil, err
	}
	if c.Domain == "" {
		c.Domain = "icloud.com"
	}
	if c.DefaultJobs < 1 {
		c.DefaultJobs = 1
	}
	if c.DefaultWait < 0 {
		c.DefaultWait = 0
	}
	return c, nil
}

// Snapshot 是脱锁的可读副本。
type Snapshot struct {
	APIKey      string `json:"api_key"`
	Proxy       string `json:"proxy"`
	Domain      string `json:"domain"`
	DefaultJobs int    `json:"default_jobs"`
	DefaultWait int    `json:"default_wait"`
	AddPassword bool   `json:"add_password"`
	AddMFA      bool   `json:"add_mfa"`
}

// Get 取一份快照，调用方可安全读取。
func (c *Config) Get() Snapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return Snapshot{
		APIKey:      c.APIKey,
		Proxy:       c.Proxy,
		Domain:      c.Domain,
		DefaultJobs: c.DefaultJobs,
		DefaultWait: c.DefaultWait,
		AddPassword: c.AddPassword,
		AddMFA:      c.AddMFA,
	}
}

// Update 整体覆盖并落盘。
func (c *Config) Update(next Snapshot) error {
	c.mu.Lock()
	c.APIKey = next.APIKey
	c.Proxy = next.Proxy
	c.Domain = next.Domain
	c.DefaultJobs = next.DefaultJobs
	c.DefaultWait = next.DefaultWait
	c.AddPassword = next.AddPassword
	c.AddMFA = next.AddMFA
	c.mu.Unlock()
	return c.save()
}

func (c *Config) save() error {
	c.mu.RLock()
	raw, err := json.MarshalIndent(c, "", "  ")
	c.mu.RUnlock()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(c.path, raw, 0o600)
}

// Mask 把密钥留头尾，代理只留主机端口。
func Mask(s string) string {
	if s == "" {
		return ""
	}
	r := []rune(s)
	if len(r) <= 8 {
		return "****"
	}
	return string(r[:4]) + "…" + string(r[len(r)-4:])
}
