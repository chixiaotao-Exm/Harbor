package store

import "context"

// PriceInfo 是一条账号的查价结果。
type PriceInfo struct {
	Country  string
	Currency string
	Plus     float64
	Pro      float64
	At       int64
}

// SetAccountPrice 保存查价结果，账号页据此标注地区与价格。
func (s *Store) SetAccountPrice(ctx context.Context, id int64, p PriceInfo) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE accounts SET price_country=?, price_currency=?, price_plus=?,
			price_pro=?, price_at=?, updated_at=? WHERE id=?`,
		p.Country, p.Currency, p.Plus, p.Pro, p.At, now(), id)
	return affected(res, err)
}

// AccountPrice 读回某账号的查价结果。
func (s *Store) AccountPrice(ctx context.Context, id int64) (PriceInfo, error) {
	var p PriceInfo
	err := s.db.QueryRowContext(ctx,
		`SELECT price_country, price_currency, price_plus, price_pro, price_at
		 FROM accounts WHERE id=?`, id).
		Scan(&p.Country, &p.Currency, &p.Plus, &p.Pro, &p.At)
	return p, err
}
