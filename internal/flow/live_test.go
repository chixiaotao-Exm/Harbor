//go:build live

package flow

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/chixi/harbor/internal/mailbox"
	"github.com/chixi/harbor/internal/netx"
)

// 跑法：
//
//	HARBOR_KEY=xxx HARBOR_PROXY=socks5://... go test -tags=live -run TestLiveRegister -v ./internal/flow/
func TestLiveRegister(t *testing.T) {
	key := os.Getenv("HARBOR_KEY")
	proxy := os.Getenv("HARBOR_PROXY")
	if key == "" || proxy == "" {
		t.Skip("需要 HARBOR_KEY 与 HARBOR_PROXY")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	marker := netx.NewMarker(func(s string) { t.Log(s) })
	exit, err := marker.Acquire(ctx, proxy)
	if err != nil {
		t.Fatalf("拿出口: %v", err)
	}
	t.Logf("出口 %s", exit.IP)

	mb := mailbox.New(key, "icloud.com", "")
	email, order, err := mb.Acquire(ctx)
	if err != nil {
		t.Fatalf("取号: %v", err)
	}
	t.Logf("邮箱 %s", email)
	defer mb.Status(ctx, order, 3)

	s, err := New(ctx, exit.Proxy)
	if err != nil {
		t.Fatalf("建会话: %v", err)
	}
	defer s.Close()

	d := Deps{
		WaitCode: func(ctx context.Context, id int64, timeout time.Duration) string {
			return mb.WaitCode(ctx, id, timeout, func() { t.Log("等验证码…") })
		},
		MarkNext: func(ctx context.Context, id int64) { mb.Status(ctx, id, 5) },
		Log:      t.Logf,
	}

	res, err := Register(ctx, s, d, Request{Email: email, OrderID: order, SetPassword: true, EnableMFA: true})
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	if res.Email != email {
		t.Fatalf("邮箱不一致")
	}
	if res.Token == "" {
		t.Fatal("未拿到登录态")
	}
	if !res.HasPassword {
		t.Fatal("密码未写入")
	}
	if res.Secret == "" {
		t.Fatal("双因子未绑定")
	}
	t.Logf("成功 %s----%s----%s", res.Email, res.Password, res.Secret)
}
