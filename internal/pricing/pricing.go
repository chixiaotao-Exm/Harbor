// Package pricing 查各国家的订阅价格与优惠。
//
// 数据来自 chatgpt.com 的结账定价配置，按国家代码取：
//
//	GET https://chatgpt.com/backend-api/checkout_pricing_config/configs/{CC}
//
// 该接口不需要登录，仅需设备号；返回该国货币、各档月/年价与生效中的优惠。
package pricing

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	http "github.com/bogdanfinn/fhttp"

	"github.com/chixi/harbor/internal/netx"
)

const baseURL = "https://chatgpt.com/backend-api/checkout_pricing_config/configs/"

// cacheTTL 价格不会频繁变，缓存十分钟足够，也避免反复打对方接口。
const cacheTTL = 10 * time.Minute

// Amount 是一档价格。
type Amount struct {
	Month float64 `json:"month,omitempty"`
	Year  float64 `json:"year,omitempty"`
}

// Promo 是一条优惠。
type Promo struct {
	Name    string  `json:"name"`
	Enabled bool    `json:"enabled"`
	Type    string  `json:"type,omitempty"`
	Amount  float64 `json:"amount,omitempty"`
}

// Price 是某个国家的价格快照。
type Price struct {
	Country  string            `json:"country"`
	Currency string            `json:"currency"`
	Symbol   string            `json:"symbol"`
	Plans    map[string]Amount `json:"plans"`
	Promos   []Promo           `json:"promos"`
	Fetched  int64             `json:"fetched_at"`
	Error    string            `json:"error,omitempty"`
}

// Client 带缓存的查价器。
type Client struct {
	proxy string
	mu    sync.Mutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	p  Price
	at time.Time
}

// New 造一个查价器。proxy 为空表示直连。
func New(proxy string) *Client {
	return &Client{proxy: proxy, cache: map[string]cacheEntry{}}
}

// SetProxy 更新出口，设置页改动后调用。
func (c *Client) SetProxy(proxy string) {
	c.mu.Lock()
	c.proxy = proxy
	c.cache = map[string]cacheEntry{}
	c.mu.Unlock()
}

// Get 取一个国家价格，命中缓存直接返回。
func (c *Client) Get(ctx context.Context, cc string) (Price, error) {
	cc = strings.ToUpper(strings.TrimSpace(cc))
	if cc == "" {
		return Price{}, fmt.Errorf("国家代码为空")
	}
	if !netx.ValidRegion(cc) || cc == netx.RandCode {
		return Price{}, fmt.Errorf("未知国家代码 %s", cc)
	}

	c.mu.Lock()
	if e, ok := c.cache[cc]; ok && time.Since(e.at) < cacheTTL {
		c.mu.Unlock()
		return e.p, nil
	}
	proxy := c.proxy
	c.mu.Unlock()

	p, err := c.fetch(ctx, cc, proxy)
	if err != nil {
		return Price{Country: cc, Error: err.Error()}, err
	}
	c.mu.Lock()
	c.cache[cc] = cacheEntry{p: p, at: time.Now()}
	c.mu.Unlock()
	return p, nil
}

// Scan 并发查一批国家，按输入顺序返回；单个失败只标记该行。
func (c *Client) Scan(ctx context.Context, ccs []string, workers int) []Price {
	if workers < 1 {
		workers = 6
	}
	out := make([]Price, len(ccs))
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for i, cc := range ccs {
		wg.Add(1)
		go func(i int, cc string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			p, err := c.Get(ctx, cc)
			if err != nil {
				p = Price{Country: strings.ToUpper(cc), Error: err.Error()}
			}
			out[i] = p
		}(i, cc)
	}
	wg.Wait()
	return out
}

func (c *Client) fetch(ctx context.Context, cc, proxy string) (Price, error) {
	cl, err := netx.NewClient(proxy, "")
	if err != nil {
		return Price{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+cc, nil)
	if err != nil {
		return Price{}, err
	}
	req.Header.Set("accept", "*/*")
	req.Header.Set("oai-device-id", deviceID())
	req.Header.Set("oai-language", "zh-CN")

	resp, err := cl.Do(req)
	if err != nil {
		return Price{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Price{}, err
	}
	if resp.StatusCode != 200 {
		return Price{}, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return parse(cc, raw)
}

// 上游结构只取用到的字段，剩下忽略。
type wire struct {
	Country        string `json:"country_code"`
	CurrencyConfig struct {
		Symbol     string `json:"symbol"`
		SymbolCode string `json:"symbol_code"`
		Plus       struct {
			Month struct{ Amount float64 } `json:"month"`
			Year  struct{ Amount float64 } `json:"year"`
		} `json:"plus"`
		Go struct {
			Month struct{ Amount float64 } `json:"month"`
		} `json:"go"`
		Pro struct {
			Month struct{ Amount float64 } `json:"month"`
		} `json:"pro"`
		Business struct {
			Month struct{ Amount float64 } `json:"month"`
			Year  struct{ Amount float64 } `json:"year"`
		} `json:"business"`
		Promos map[string]struct {
			Enabled bool    `json:"enabled"`
			Type    string  `json:"type"`
			Amount  float64 `json:"amount"`
		} `json:"promos"`
	} `json:"currency_config"`
}

func parse(cc string, raw []byte) (Price, error) {
	var w wire
	if err := json.Unmarshal(raw, &w); err != nil {
		return Price{}, fmt.Errorf("解析失败: %w", err)
	}
	p := Price{
		Country:  firstNonEmpty(w.Country, cc),
		Currency: w.CurrencyConfig.SymbolCode,
		Symbol:   w.CurrencyConfig.Symbol,
		Plans:    map[string]Amount{},
		Fetched:  time.Now().Unix(),
	}
	set := func(name string, a Amount) {
		if a.Month > 0 || a.Year > 0 {
			p.Plans[name] = a
		}
	}
	set("plus", Amount{
		Month: w.CurrencyConfig.Plus.Month.Amount,
		Year:  w.CurrencyConfig.Plus.Year.Amount,
	})
	set("go", Amount{Month: w.CurrencyConfig.Go.Month.Amount})
	set("pro", Amount{Month: w.CurrencyConfig.Pro.Month.Amount})
	set("business", Amount{
		Month: w.CurrencyConfig.Business.Month.Amount,
		Year:  w.CurrencyConfig.Business.Year.Amount,
	})

	for name, v := range w.CurrencyConfig.Promos {
		p.Promos = append(p.Promos, Promo{Name: name, Enabled: v.Enabled, Type: v.Type, Amount: v.Amount})
	}
	sortPromos(p.Promos)
	return p, nil
}

// sortPromos 让输出稳定，便于比对。
func sortPromos(list []Promo) {
	for i := 1; i < len(list); i++ {
		for j := i; j > 0 && list[j].Name < list[j-1].Name; j-- {
			list[j], list[j-1] = list[j-1], list[j]
		}
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// deviceID 每次进程内固定一个，便于对方做限流统计。
var deviceID = func() func() string {
	var once sync.Once
	var id string
	return func() string {
		once.Do(func() { id = randomUUID() })
		return id
	}
}()

func randomUUID() string {
	const hex = "0123456789abcdef"
	b := make([]byte, 32)
	for i := range b {
		b[i] = hex[randInt(16)]
	}
	b[12] = '4'
	b[16] = "89ab"[randInt(4)]
	return fmt.Sprintf("%s-%s-%s-%s-%s", b[0:8], b[8:12], b[12:16], b[16:20], b[20:32])
}
