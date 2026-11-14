package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/chixi/harbor/internal/netx"
	"github.com/chixi/harbor/internal/pricing"
	"github.com/chixi/harbor/internal/store"
)

// handleAccountPrice 给一批账号查优惠价，结果写回账号页。
//
// 地区取自账号出口代理里的 region 段；随机出口就按当前设置的地区兜底。
func (s *Server) handleAccountPrice(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []int64 `json:"ids"`
		All bool    `json:"all"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体不是合法 JSON")
		return
	}
	ids := in.IDs
	if in.All || len(ids) == 0 {
		ids = nil
		for _, kind := range []string{"mfa", "plain"} {
			part, err := s.Store.AllIDs(r.Context(), kind, "")
			if err != nil {
				s.fail(w, err)
				return
			}
			ids = append(ids, part...)
		}
	}

	cl := s.priceClient(r)
	st, _ := s.Store.GetSettings(r.Context())

	type result struct {
		ID       int64   `json:"id"`
		Email    string  `json:"email"`
		Country  string  `json:"country"`
		Currency string  `json:"currency"`
		Plus     float64 `json:"plus"`
		Pro      float64 `json:"pro"`
		Error    string  `json:"error,omitempty"`
	}
	out := make([]result, 0, len(ids))
	ok, failed := 0, 0

	for _, id := range ids {
		acc, err := s.Store.GetAccount(r.Context(), id)
		if err != nil {
			failed++
			continue
		}
		cc := netx.RegionOf(acc.Proxy)
		if cc == "" || cc == netx.RandCode {
			cc = st.Region
		}
		row := result{ID: id, Email: acc.Email, Country: cc}
		if cc == "" || cc == netx.RandCode {
			row.Error = "地区未知"
			out = append(out, row)
			failed++
			continue
		}

		p, err := cl.Get(r.Context(), cc)
		if err != nil {
			row.Error = err.Error()
			out = append(out, row)
			failed++
			continue
		}
		row.Currency = p.Currency
		row.Plus = amount(p, "plus")
		row.Pro = amount(p, "pro")
		_ = s.Store.SetAccountPrice(r.Context(), id, store.PriceInfo{
			Country: cc, Currency: p.Currency, Plus: row.Plus, Pro: row.Pro,
			At: time.Now().Unix(),
		})
		out = append(out, row)
		ok++
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok": ok, "failed": failed, "rows": out,
	})
}

// priceClient 复用带缓存的查价器。
func (s *Server) priceClient(*http.Request) *pricing.Client {
	if s.Price == nil {
		s.Price = pricing.New("")
	}
	return s.Price
}

// amount 取某档月价，缺月价就退回年价。
func amount(p pricing.Price, plan string) float64 {
	a, ok := p.Plans[plan]
	if !ok {
		return 0
	}
	if a.Month > 0 {
		return a.Month
	}
	return a.Year
}

var _ = context.Background
