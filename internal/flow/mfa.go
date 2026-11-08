package flow

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	http "github.com/bogdanfinn/fhttp"
)

// TOTPCode 由密钥算出当前动态码。
func TOTPCode(secret string) (string, error) {
	norm := strings.ToUpper(strings.ReplaceAll(secret, " ", ""))
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(norm)
	if err != nil {
		return "", err
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(time.Now().Unix()/30))
	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	code := (uint32(sum[off])&0x7f)<<24 |
		uint32(sum[off+1])<<16 |
		uint32(sum[off+2])<<8 |
		uint32(sum[off+3])
	return fmt.Sprintf("%06d", code%1000000), nil
}

func mfaHdr(token, referer string) http.Header {
	h := http.Header{}
	h.Set("authorization", "Bearer "+token)
	h.Set("origin", "https://chatgpt.com")
	h.Set("referer", referer)
	h.Set("content-type", "application/json")
	h.Set("accept", "application/json")
	return h
}

func (s *Session) backend(ctx context.Context, path string, body any, token string) (int, []byte, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return 0, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://chatgpt.com"+path, strings.NewReader(string(raw)))
	if err != nil {
		return 0, nil, err
	}
	req.Header = mfaHdr(token, "https://chatgpt.com/settings/security")
	resp, err := s.Client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	out := make([]byte, 0, 4096)
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		out = append(out, buf[:n]...)
		if err != nil || n == 0 || len(out) > 1<<20 {
			break
		}
	}
	return resp.StatusCode, out, nil
}

// EnableMFA 登记并激活双因子，返回密钥。
func EnableMFA(ctx context.Context, s *Session, token string) (string, error) {
	if token == "" {
		return "", errors.New("没有登录态，无法绑双因子")
	}
	code, raw, err := s.backend(ctx, "/backend-api/accounts/mfa/enroll",
		map[string]string{"factor_type": "totp", "source": "settings"}, token)
	if err != nil {
		return "", err
	}
	if code != 200 {
		return "", fmt.Errorf("登记 HTTP %d: %s", code, snippet(raw))
	}
	var enroll struct {
		Secret    string `json:"secret"`
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(raw, &enroll); err != nil || enroll.Secret == "" {
		return "", errors.New("登记返回缺少密钥")
	}

	otp, err := TOTPCode(enroll.Secret)
	if err != nil {
		return "", err
	}
	code, raw, err = s.backend(ctx, "/backend-api/accounts/mfa/user/activate_enrollment",
		map[string]string{
			"factor_type": "totp",
			"session_id":  enroll.SessionID,
			"code":        otp,
			"source":      "settings",
		}, token)
	if err != nil {
		return "", err
	}
	if code != 200 {
		return "", fmt.Errorf("激活 HTTP %d: %s", code, snippet(raw))
	}

	_, _, _ = s.backend(ctx, "/backend-api/accounts/mfa/factor_decision",
		map[string]string{"entrypoint": "post_totp"}, token)
	return enroll.Secret, nil
}

// ProbeSession 校验登录态是否可用，返回是否有效。
func ProbeSession(ctx context.Context, s *Session, token string) bool {
	if token == "" {
		return false
	}
	s.SetToken(token)
	_, raw, err := s.do(ctx, http.MethodGet, "https://chatgpt.com/backend-api/me", nil,
		map[string]string{"authorization": "Bearer " + token, "accept": "application/json"})
	if err != nil {
		return false
	}
	return strings.Contains(string(raw), `"email"`) || strings.Contains(string(raw), `"id"`)
}
