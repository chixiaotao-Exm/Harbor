// Package mailbox 封装临时邮箱与收码，并保证取号全局串行。
package mailbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	http "github.com/bogdanfinn/fhttp"

	"github.com/chixi/harbor/internal/netx"
)

const (
	base    = "https://smsbower.page"
	service = "dr"
)

// Domains 是可选域名，按顺序降级。
var Domains = []string{"icloud.com", "gmail.com", "outlook.com"}

// ErrNoMail 表示平台暂时没货。
var ErrNoMail = errors.New("平台没有可用邮箱")

var (
	// 全局取号锁：一次只允许一条任务取号。
	acquireMu sync.Mutex

	codeRe = regexp.MustCompile(`\d{6}`)
)

// Client 是接码平台客户端。
type Client struct {
	Key    string
	Domain string
	ua     string
}

// New 造一个客户端。
func New(key, domain, ua string) *Client {
	if domain == "" {
		domain = Domains[0]
	}
	return &Client{Key: key, Domain: domain, ua: ua}
}

type envelope struct {
	Status int    `json:"status"`
	Error  string `json:"error"`
	Mail   string `json:"mail"`
	MailID int64  `json:"mailId"`
	Code   string `json:"code"`
}

func (c *Client) call(ctx context.Context, path string, params map[string]string) (envelope, error) {
	q := url.Values{}
	q.Set("api_key", c.Key)
	for k, v := range params {
		q.Set(k, v)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path+"?"+q.Encode(), nil)
	if err != nil {
		return envelope{}, err
	}
	if c.ua != "" {
		req.Header.Set("user-agent", c.ua)
	}
	cl, err := netx.NewClient("", c.ua)
	if err != nil {
		return envelope{}, err
	}
	resp, err := cl.Do(req)
	if err != nil {
		return envelope{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if err != nil {
		return envelope{}, err
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return envelope{}, fmt.Errorf("取号接口返回异常: %s", strings.TrimSpace(string(raw)))
	}
	return env, nil
}

// Balance 查余额原文。
func (c *Client) Balance(ctx context.Context) (string, error) {
	q := url.Values{"api_key": {c.Key}, "action": {"getBalance"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		base+"/stubs/handler_api.php?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	cl, err := netx.NewClient("", c.ua)
	if err != nil {
		return "", err
	}
	resp, err := cl.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
	return strings.TrimSpace(string(raw)), err
}

// Acquire 取一个邮箱，串行执行，指定域名没货时自动降级。
//
// 锁覆盖整个取号过程，保证并发 N 条任务时也不会同时打平台。
func (c *Client) Acquire(ctx context.Context) (email string, id int64, err error) {
	acquireMu.Lock()
	defer acquireMu.Unlock()

	order := append([]string{c.Domain}, without(Domains, c.Domain)...)
	var last error
	for _, domain := range order {
		env, callErr := c.call(ctx, "/api/mail/getActivation", map[string]string{
			"service": service, "domain": domain,
		})
		if callErr != nil {
			last = callErr
			continue
		}
		if env.Status == 1 && env.Mail != "" && env.MailID > 0 {
			c.Domain = domain
			return env.Mail, env.MailID, nil
		}
		last = fmt.Errorf("%w: %s -> %s", ErrNoMail, domain, env.Error)
		if strings.Contains(env.Error, "No mails") {
			continue // 换下一个域名
		}
	}
	if last == nil {
		last = ErrNoMail
	}
	return "", 0, last
}

// WaitCode 轮询验证码，超时返回空串。
func (c *Client) WaitCode(ctx context.Context, id int64, timeout time.Duration, onWait func()) string {
	deadline := time.Now().Add(timeout)
	next := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		env, err := c.call(ctx, "/api/mail/getCode", map[string]string{"mailId": itoa(id)})
		if err == nil && env.Status == 1 {
			if m := codeRe.FindString(env.Code); m != "" {
				return m
			}
		}
		if onWait != nil && time.Now().After(next) {
			next = time.Now().Add(15 * time.Second)
			onWait()
		}
		select {
		case <-ctx.Done():
			return ""
		case <-time.After(4 * time.Second):
		}
	}
	return ""
}

// Status 标记订单状态：3 完成、2 取消、5 等下一条码。
func (c *Client) Status(ctx context.Context, id int64, status int) {
	_, _ = c.call(ctx, "/api/mail/setStatus", map[string]string{
		"id": itoa(id), "status": itoa(int64(status)),
	})
}

func without(list []string, skip string) []string {
	out := make([]string, 0, len(list))
	for _, v := range list {
		if v != skip {
			out = append(out, v)
		}
	}
	return out
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	var b []byte
	neg := v < 0
	if neg {
		v = -v
	}
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
