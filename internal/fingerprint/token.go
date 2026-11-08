package fingerprint

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
)

// Server 是对方下发的一次性挑战。
type Server struct {
	Token   string `json:"token"`
	Persona string `json:"persona"`
	Proof   struct {
		Required   bool   `json:"required"`
		Seed       string `json:"seed"`
		Difficulty string `json:"difficulty"`
	} `json:"proofofwork"`
	SO struct {
		CollectorDX string `json:"collector_dx"`
	} `json:"so"`
}

// Token 是要回送的凭据体。
type Token struct {
	P    string `json:"p"`
	T    string `json:"t"`
	C    string `json:"c"`
	ID   string `json:"id"`
	Flow string `json:"flow"`
}

// Build 组一份凭据。有难度就现场算证明，answer 回填进数组第 3 位。
func Build(p Profile, srv Server, flow string) (Token, error) {
	arr := p.Arr()
	if srv.Proof.Required && srv.Proof.Difficulty != "" {
		solved, err := Solve(arr, srv.Proof.Seed, srv.Proof.Difficulty)
		if err != nil {
			return Token{}, err
		}
		arr = solved
	}

	plain, err := EncodePlain(arr)
	if err != nil {
		return Token{}, err
	}
	payload, err := Encode(arr, randSeed())
	if err != nil {
		return Token{}, err
	}
	id := srv.Token
	if id == "" {
		id = p.DeviceID
	}
	return Token{P: payload, T: plain, C: srv.Token, ID: p.DeviceID, Flow: flow}, nil
}

// Marshal 序列化成头值。
func (t Token) Marshal() (string, error) {
	raw, err := json.Marshal(t)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// SOHeader 组会话观察头。
func SOHeader(srv Server, deviceID, flow string) (string, error) {
	if srv.SO.CollectorDX == "" {
		return "", nil
	}
	raw, err := json.Marshal(map[string]string{
		"so": srv.SO.CollectorDX, "c": srv.Token, "id": deviceID, "flow": flow,
	})
	if err != nil {
		return "", nil
	}
	return string(raw), nil
}

// ChallengeBody 组挑战请求体。
func ChallengeBody(p Profile) (string, error) {
	plain, err := EncodePlain(p.Arr())
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(map[string]string{
		"p": plain, "id": p.DeviceID, "flow": "email_otp_validate",
	})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func randSeed() byte {
	v, err := rand.Int(rand.Reader, big.NewInt(255))
	if err != nil {
		return 0
	}
	return byte(v.Int64())
}

// Describe 给日志用的一行摘要。
func (p Profile) Describe() string {
	return fmt.Sprintf("%dx%d %s", p.Width, p.Height, p.Lang)
}
