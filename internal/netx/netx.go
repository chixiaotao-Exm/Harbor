// Package netx handles proxy strings, sid rotation and exit probing.
package netx

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/url"
	"regexp"
	"strings"
	"time"

	http "github.com/bogdanfinn/fhttp"
	"github.com/bogdanfinn/fhttp/cookiejar"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

// ErrNoExit means probing never produced a usable exit.
var ErrNoExit = errors.New("no usable exit")

var probeURLs = []string{
	"https://api.ipify.org?format=json",
	"https://ifconfig.me/ip",
}

const (
	maxProbeRounds = 6
	jitterBase     = 450 * time.Millisecond
)

var (
	regionPattern = regexp.MustCompile(`(-region-)([A-Za-z]{2})`)
	sidPattern    = regexp.MustCompile(`(-(?:sid|session)-)([A-Za-z0-9]+)`)
	wsPattern     = regexp.MustCompile(`\s+`)
	ipv4Pattern   = regexp.MustCompile(`^\d{1,3}(\.\d{1,3}){3}$`)
)

const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// Normalize cleans a pasted proxy string: drop every whitespace char
// (curl rejects inner spaces), fold full-width chars, and add a scheme.
func Normalize(proxy string) string {
	p := strings.TrimSpace(proxy)
	if p == "" {
		return ""
	}
	p = strings.NewReplacer(
		"：", ":", "／", "/", "＠", "@", "．", ".",
		"\u00a0", "", "\u200b", "",
	).Replace(p)
	p = wsPattern.ReplaceAllString(p, "")
	p = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, p)
	if p == "" {
		return ""
	}

	low := strings.ToLower(p)
	switch {
	case strings.HasPrefix(low, "socks5://"),
		strings.HasPrefix(low, "socks5h://"),
		strings.HasPrefix(low, "socks4://"),
		strings.HasPrefix(low, "http://"),
		strings.HasPrefix(low, "https://"):
		return p
	case strings.Contains(p, "@") && !strings.Contains(p, "://"):
		return "socks5h://" + p
	case !strings.Contains(p, "://"):
		return "http://" + p
	}
	return p
}

// WithRegion rewrites the region segment (for example -region-US-) in place.
// An empty or unknown code leaves the proxy untouched.
func WithRegion(proxy, code string) string {
	p := Normalize(proxy)
	if p == "" || code == "" {
		return p
	}
	code = strings.ToUpper(strings.TrimSpace(code))
	if !ValidRegion(code) {
		return p
	}
	return regionPattern.ReplaceAllString(p, `${1}`+code)
}

// RegionOf reads the region code out of a proxy, empty when absent.
func RegionOf(proxy string) string {
	m := regionPattern.FindStringSubmatch(proxy)
	if len(m) < 3 {
		return ""
	}
	return strings.ToUpper(m[2])
}

// Regions are the country codes the UI offers.
var Regions = []string{"US", "GB", "JP", "DE", "SG", "CA", "AU", "NL", "FR"}

// ValidRegion reports whether code is in Regions.
func ValidRegion(code string) bool {
	for _, r := range Regions {
		if r == strings.ToUpper(code) {
			return true
		}
	}
	return false
}

// Rotate rewrites the sid / session segment with a fresh token.
func Rotate(proxy string) string {
	out := Normalize(proxy)
	if out == "" {
		return ""
	}
	out = sidPattern.ReplaceAllStringFunc(out, func(m string) string {
		sub := sidPattern.FindStringSubmatch(m)
		return sub[1] + randToken(len(sub[2]))
	})
	if strings.HasPrefix(out, "socks5://") {
		out = "socks5h://" + strings.TrimPrefix(out, "socks5://")
	}
	return out
}

func randToken(n int) string {
	if n <= 0 {
		n = 8
	}
	var b strings.Builder
	b.Grow(n)
	for i := 0; i < n; i++ {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			b.WriteByte(alphabet[i%len(alphabet)])
			continue
		}
		b.WriteByte(alphabet[idx.Int64()])
	}
	return b.String()
}

// HostOf returns the host:port tail of a proxy, for logs.
func HostOf(proxy string) string {
	if i := strings.LastIndex(proxy, "@"); i >= 0 {
		return proxy[i+1:]
	}
	if u, err := url.Parse(proxy); err == nil {
		return u.Host
	}
	return proxy
}

// NewClient builds a fingerprinting client with a cookie jar and proxy.
func NewClient(proxy string, ua string) (tls_client.HttpClient, error) {
	p := Normalize(proxy)
	jar, _ := cookiejar.New(nil)
	opts := []tls_client.HttpClientOption{
		tls_client.WithClientProfile(profiles.Chrome_133),
		tls_client.WithTimeoutSeconds(45),
		tls_client.WithRandomTLSExtensionOrder(),
		tls_client.WithCookieJar(jar),
	}
	if p != "" {
		opts = append(opts, tls_client.WithProxyUrl(p))
	}
	if ua != "" {
		opts = append(opts, tls_client.WithConnectHeaders(http.Header{"User-Agent": {ua}}))
	}
	return tls_client.NewHttpClient(tls_client.NewNoopLogger(), opts...)
}

// Exit is one usable exit.
type Exit struct {
	Proxy string
	IP    string
}

// Marker probes exits; functions are swappable in tests.
type Marker struct {
	Probe func(ctx context.Context, proxy string) (string, error)
	Sleep func(d time.Duration)
	Log   func(string)
}

// NewMarker wires the real network probe.
func NewMarker(log func(string)) *Marker {
	return &Marker{Probe: Probe, Sleep: time.Sleep, Log: log}
}

// Acquire rotates sid and probes until an exit answers.
func (m *Marker) Acquire(ctx context.Context, proxy string) (Exit, error) {
	base := Normalize(proxy)
	if base == "" {
		return Exit{}, fmt.Errorf("%w: empty proxy", ErrNoExit)
	}
	var last error
	for round := 1; round <= maxProbeRounds; round++ {
		if err := ctx.Err(); err != nil {
			return Exit{}, err
		}
		cand := Rotate(base)
		ip, err := m.Probe(ctx, cand)
		if err == nil && ip != "" {
			return Exit{Proxy: cand, IP: ip}, nil
		}
		last = err
		if m.Log != nil {
			m.Log(fmt.Sprintf("exit probe failed (%d/%d) %s: %v", round, maxProbeRounds, HostOf(cand), err))
		}
		if m.Sleep != nil {
			m.Sleep(jitterBase * time.Duration(round))
		}
	}
	if last == nil {
		last = errors.New("probe returned nothing")
	}
	return Exit{}, fmt.Errorf("%w: %v", ErrNoExit, last)
}

// Probe hits a probe URL through the proxy and returns the reported IP.
func Probe(ctx context.Context, proxy string) (string, error) {
	c, err := NewClient(proxy, "")
	if err != nil {
		return "", err
	}
	var last error
	for _, target := range probeURLs {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			last = err
			continue
		}
		req.Header.Set("accept", "application/json, text/plain, */*")
		resp, err := c.Do(req)
		if err != nil {
			last = err
			continue
		}
		raw, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		if err != nil {
			last = err
			continue
		}
		if resp.StatusCode != 200 {
			last = fmt.Errorf("probe HTTP %d", resp.StatusCode)
			continue
		}
		if ip := pickIP(raw); ip != "" {
			return ip, nil
		}
		last = errors.New("no ip in probe body")
	}
	if last == nil {
		last = errors.New("probe failed")
	}
	return "", last
}

func pickIP(raw []byte) string {
	var m struct {
		IP string `json:"ip"`
	}
	if json.Unmarshal(raw, &m) == nil && m.IP != "" {
		return m.IP
	}
	s := strings.TrimSpace(string(raw))
	if ipv4Pattern.MatchString(s) {
		return s
	}
	if strings.Contains(s, ":") && len(s) < 46 && !strings.ContainsAny(s, " \n") {
		return s
	}
	return ""
}
