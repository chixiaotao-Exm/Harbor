package store

import "context"

// SetSecret 写入双因子密钥。
func (s *Store) SetSecret(ctx context.Context, id int64, secret string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE accounts SET totp_secret=?, has_password=1, updated_at=? WHERE id=?`,
		secret, now(), id)
	return affected(res, err)
}
