// Package fingerprint 生成每号独立的浏览器档案与放行凭据。
//
// 载荷格式： "gAAAAAB" + base64(明文数组 ^ seed) + "~S"，seed 为单字节。
// 工作量证明：对 seed+answer 做 FNV-1a 32 再混合，取 hex 前缀与难度串字典序比较。
package fingerprint

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"
)

const (
	sdkPath    = "/sentinel/20260810913b/sdk.js"
	sdkBase    = "https://sentinel.openai.com"
	prefix     = "gAAAAAB"
	suffix     = "~S"
	maxTries   = 500000
	uaTemplate = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%d.0.%d.%d Safari/537.36"
)

// SDKURL 是档案里上报的脚本地址。
var SDKURL = sdkBase + sdkPath

// Profile 是一套指纹。全部值现场随机，账号之间不同源。
type Profile struct {
	Width      int
	Height     int
	UA         string
	Lang       string
	Languages  string
	Webdriver  int
	ScreenName string
	GlobalName string
	EventName  string
	TimeOrigin float64
	NowBase    float64
	FirstReq   float64
	DeviceID   string
	Flags      map[string]int
}

var (
	widths    = []int{1280, 1366, 1440, 1536, 1600, 1920, 2560}
	heights   = []int{720, 768, 800, 864, 900, 1080, 1200, 1440}
	languages = []string{
		"en-US,en", "en-GB,en", "en-US,en;q=0.9,zh-CN;q=0.8",
		"en-CA,en;q=0.9,fr-CA;q=0.8", "de-DE,de;q=0.9,en;q=0.8",
		"fr-FR,fr;q=0.9,en;q=0.8", "en-AU,en;q=0.9",
	}
	screenNames = []string{"serial−[object Serial]", "availWidth", "availTop", "devicePixelRatio"}
	globalNames = []string{"location", "onLine", "name", "status"}
	eventNames  = []string{"resizeTo", "online", "offline", "orientationchange"}
	zones       = []string{"China Standard Time", "Eastern Standard Time", "Pacific Standard Time", "Central European Summer Time"}
	offsets     = []string{"GMT-0400", "GMT-0500", "GMT+0800", "GMT+0200"}
)

// New 造一套随机指纹。
func New() Profile {
	p := Profile{
		Width:      pick(widths),
		Height:     pick(heights),
		Languages:  pick(languages),
		Webdriver:  pick([]int{4, 6, 8}),
		ScreenName: pick(screenNames),
		GlobalName: pick(globalNames),
		EventName:  pick(eventNames),
		TimeOrigin: round(randFloat(2000, 60000)),
		NowBase:    round(randFloat(10000, 40000)),
		FirstReq:   round(float64(time.Now().UnixMilli()) + randFloat(0, 500)),
		DeviceID:   uuid(),
		Flags: map[string]int{
			"ai":             1,
			"cache":          1,
			"dataBuild":      pick([]int{0, 1}),
			"solana":         pick([]int{0, 1}),
			"installTrigger": 0,
		},
	}
	p.Lang = strings.Split(strings.Split(p.Languages, ",")[0], ";")[0]
	chrome := 126 + randInt(15)
	build := 6000 + randInt(900)
	p.UA = fmt.Sprintf(uaTemplate, chrome, build, 100+randInt(120))
	return p
}

// Arr 产出上报数组，下标含义与上游一致。第 3 位留 0，由 Solve 回填。
func (p Profile) Arr() []any {
	return []any{
		fmt.Sprintf("%d%d", p.Width, p.Height),
		p.dateString(),
		p.TimeOrigin,
		0,
		p.UA,
		SDKURL,
		nil,
		p.Lang,
		p.Languages,
		p.Webdriver,
		p.ScreenName,
		p.GlobalName,
		p.EventName,
		p.NowBase,
		p.DeviceID,
		"",
		24,
		p.FirstReq,
		p.Flags["ai"],
		p.Flags["cache"],
		p.Flags["dataBuild"],
		p.Flags["solana"],
		p.Flags["installTrigger"],
		0,
		0,
	}
}

// Headers 给出与该档案匹配的请求头。
func (p Profile) Headers() map[string]string {
	chrome := "133"
	if i := strings.Index(p.UA, "Chrome/"); i >= 0 {
		rest := p.UA[i+len("Chrome/"):]
		if j := strings.Index(rest, "."); j > 0 {
			chrome = rest[:j]
		}
	}
	return map[string]string{
		"User-Agent":         p.UA,
		"Accept-Language":    p.Languages,
		"sec-ch-ua":          fmt.Sprintf(`"Chromium";v="%s", "Not;A=Brand";v="99", "Google Chrome";v="%s"`, chrome, chrome),
		"sec-ch-ua-mobile":   "?0",
		"sec-ch-ua-platform": `"Windows"`,
	}
}

func (p Profile) dateString() string {
	return time.Now().Format("Mon Jan 02 2006 15:04:05 ") + pick(offsets) + " (" + pick(zones) + ")"
}

// Solve 按难度求出满足条件的 answer，并返回回填后的数组。
func Solve(arr []any, seedStr, difficulty string) ([]any, error) {
	want := len(difficulty)
	prefixBytes := []byte(seedStr)
	for i := 0; i < maxTries; i++ {
		ans := itoa(i)
		h := fnvMurmur(append(append([]byte{}, prefixBytes...), ans...))
		if fmt.Sprintf("%08x", h)[:want] <= difficulty {
			out := make([]any, len(arr))
			copy(out, arr)
			out[3] = i
			return out, nil
		}
	}
	return nil, fmt.Errorf("工作量证明超过 %d 次仍未命中", maxTries)
}

// Encode 把数组加密成载荷。
func Encode(arr []any, seed byte) (string, error) {
	raw, err := json.Marshal(arr)
	if err != nil {
		return "", err
	}
	mixed := make([]byte, len(raw))
	for i, b := range raw {
		mixed[i] = b ^ seed
	}
	return prefix + base64.StdEncoding.EncodeToString(mixed) + suffix, nil
}

// EncodePlain 产出 t 字段用的明文数组 base64。
func EncodePlain(arr []any) (string, error) {
	raw, err := json.Marshal(arr)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

// Decode 解回数组，供校验与测试。
//
// seed 是整段单字节异或，不随载荷下发，按 0-255 试到能解析为止。
func Decode(payload string) ([]any, error) {
	body := strings.TrimSuffix(strings.TrimPrefix(payload, prefix), suffix)
	raw, err := base64.StdEncoding.DecodeString(body)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("载荷为空")
	}
	try := func(seed byte) ([]any, bool) {
		out := make([]byte, len(raw))
		for i, b := range raw {
			out[i] = b ^ seed
		}
		var arr []any
		if json.Unmarshal(out, &arr) != nil || len(arr) == 0 {
			return nil, false
		}
		return arr, true
	}
	// 种子为 0 最常见，先试。
	if arr, ok := try(0); ok {
		return arr, nil
	}
	for seed := 1; seed < 256; seed++ {
		if arr, ok := try(byte(seed)); ok {
			return arr, nil
		}
	}
	return nil, fmt.Errorf("载荷无法解出数组")
}

// SeedOf 反查载荷用的种子，测试与排障用。
func SeedOf(payload string) (byte, bool) {
	body := strings.TrimSuffix(strings.TrimPrefix(payload, prefix), suffix)
	raw, err := base64.StdEncoding.DecodeString(body)
	if err != nil || len(raw) == 0 {
		return 0, false
	}
	for seed := 0; seed < 256; seed++ {
		out := make([]byte, len(raw))
		for i, b := range raw {
			out[i] = b ^ byte(seed)
		}
		var arr []any
		if json.Unmarshal(out, &arr) == nil && len(arr) > 0 {
			return byte(seed), true
		}
	}
	return 0, false
}

func fnvMurmur(buf []byte) uint32 {
	e := uint32(2166136261)
	for _, b := range buf {
		e ^= uint32(b)
		e *= 16777619
	}
	e ^= e >> 16
	e *= 2246822507
	e ^= e >> 13
	e *= 3266489909
	e ^= e >> 16
	return e
}

func pick[T any](list []T) T { return list[randInt(len(list))] }

func randInt(n int) int {
	if n <= 0 {
		return 0
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0
	}
	return int(v.Int64())
}

func randFloat(min, max float64) float64 {
	v, err := rand.Int(rand.Reader, big.NewInt(1<<53))
	if err != nil {
		return min
	}
	f := float64(v.Int64()) / float64(int64(1)<<53)
	return min + f*(max-min)
}

func round(v float64) float64 {
	return float64(int64(v*10000)) / 10000
}

func uuid() string {
	const hex = "0123456789abcdef"
	var b [32]byte
	for i := range b {
		b[i] = hex[randInt(16)]
	}
	b[12] = '4'
	b[16] = "89ab"[randInt(4)]
	return fmt.Sprintf("%s-%s-%s-%s-%s", b[0:8], b[8:12], b[12:16], b[16:20], b[20:32])
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var b []byte
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	return string(b)
}
