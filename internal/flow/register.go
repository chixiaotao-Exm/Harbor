package flow

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/chixi/harbor/internal/fingerprint"
)

// Deps 是开户所需的外部依赖，便于测试替换。
type Deps struct {
	Acquire  func(ctx context.Context) (email string, orderID int64, err error)
	WaitCode func(ctx context.Context, orderID int64, timeout time.Duration) string
	Done     func(ctx context.Context, orderID int64)
	Cancel   func(ctx context.Context, orderID int64)
	MarkNext func(ctx context.Context, orderID int64)
	Renew    func(ctx context.Context) (proxy, ip string, err error)
	Log      func(format string, args ...any)
}

// Request 是一次开户的输入。
type Request struct {
	Email       string
	OrderID     int64
	SetPassword bool
	EnableMFA   bool
}

// Result 是开户产物。
type Result struct {
	Email       string
	Password    string
	Secret      string
	Name        string
	Birthdate   string
	Token       string
	Proxy       string
	IP          string
	HasPassword bool
}

var errNoCode = errors.New("等不到验证码")

// Register 跑完整条链路。出口在调用前已由上层固定并传入。
func Register(ctx context.Context, s *Session, d Deps, req Request) (*Result, error) {
	log := d.Log
	stage := "打开验证页"
	log("正在处理 %s —— %s", req.Email, stage)

	if err := s.Bootstrap(ctx, req.Email); err != nil {
		return nil, wrap(stage, err)
	}

	stage = "发送验证码"
	log("正在处理 %s —— %s", req.Email, stage)
	if err := s.SendOTP(ctx); err != nil {
		return nil, wrap(stage, err)
	}

	stage = "等待验证码邮件"
	log("正在处理 %s —— %s", req.Email, stage)
	code := d.WaitCode(ctx, req.OrderID, 240*time.Second)
	if code == "" {
		return nil, wrap(stage, errNoCode)
	}
	log("%s 收到验证码 %s", req.Email, code)

	stage = "校验验证码"
	page, err := s.ValidateOTP(ctx, code)
	if err != nil {
		return nil, wrap(stage, err)
	}
	log("%s 校验通过（%s）", req.Email, page)

	stage = "建档"
	name := randomName()
	birth := randomBirthdate()
	cb, err := s.CreateAccount(ctx, name, birth)
	if err != nil {
		return nil, wrap(stage, err)
	}
	log("%s 建档成功，姓名 %s", req.Email, name)

	stage = "回调取票据"
	token, err := s.Callback(ctx, cb)
	if err != nil {
		return nil, wrap(stage, err)
	}
	if token == "" {
		return nil, wrap(stage, errors.New("回调后没拿到登录态"))
	}

	res := &Result{Email: req.Email, Name: name, Birthdate: birth, Token: token}

	if req.SetPassword {
		stage = "写入密码"
		password := randomPassword()
		log("%s 开户成功，开始写入密码", req.Email)
		if d.MarkNext != nil {
			d.MarkNext(ctx, req.OrderID)
		}
		fresh, ok := s.SetPassword(ctx, req.Email, password, func() string {
			return d.WaitCode(ctx, req.OrderID, 180*time.Second)
		})
		if !ok {
			log("%s 密码写入失败，账号按免密保存", req.Email)
		} else {
			res.Password = password
			res.HasPassword = true
			if fresh != "" {
				res.Token = fresh
			}
			log("%s 密码已写入", req.Email)

			if req.EnableMFA {
				stage = "绑双因子"
				secret, err := EnableMFA(ctx, s, res.Token)
				if err != nil {
					log("%s 绑双因子失败：%v（已入失败页）", req.Email, err)
				} else {
					res.Secret = secret
					log("%s 双因子已绑定", req.Email)
				}
			}
		}
	}
	return res, nil
}

func wrap(stage string, err error) error {
	if errors.Is(err, ErrBlocked) {
		return fmt.Errorf("%s: %w", stage, err)
	}
	return fmt.Errorf("%s: %v", stage, err)
}

var firstNames = []string{"james", "olivia", "liam", "emma", "noah", "ava", "lucas", "mia", "ethan", "zoe"}

func randomName() string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	var b strings.Builder
	b.WriteString(pickStr(firstNames))
	for i := 0; i < 3+randInt(4); i++ {
		b.WriteByte(letters[randInt(len(letters))])
	}
	return b.String()
}

func randomBirthdate() string {
	return fmt.Sprintf("%04d-%02d-%02d", 1988+randInt(18), 1+randInt(12), 1+randInt(28))
}

func randomPassword() string {
	const all = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	var b strings.Builder
	for i := 0; i < 14; i++ {
		b.WriteByte(all[randInt(len(all))])
	}
	b.WriteByte("ABCDEFGHIJKLMNOPQRSTUVWXYZ"[randInt(26)])
	b.WriteByte("0123456789"[randInt(10)])
	return b.String()
}

func pickStr(list []string) string { return list[randInt(len(list))] }

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

// ProfileOf 暴露会话指纹，测试用。
func (s *Session) ProfileOf() fingerprint.Profile { return s.Profile }

// SetToken 记住外部带来的登录态（导入已有账号时用）。
func (s *Session) SetToken(token string) { s.token = token }
