package store

import "context"

// SetAccountSession 保存账号的会话快照（/api/auth/session 原文）。
func (s *Store) SetAccountSession(ctx context.Context, id int64, raw string, at int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE accounts SET session_json=?, session_at=?, updated_at=? WHERE id=?`,
		raw, at, now(), id)
	return affected(res, err)
}

// AccountSession 读回会话快照。
func (s *Store) AccountSession(ctx context.Context, id int64) (string, int64, error) {
	var raw string
	var at int64
	err := s.db.QueryRowContext(ctx,
		`SELECT session_json, session_at FROM accounts WHERE id=?`, id).Scan(&raw, &at)
	return raw, at, err
}
