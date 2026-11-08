package store

import "context"

// CountKind 数某页签的条数。
func (s *Store) CountKind(ctx context.Context, kind string) (int, error) {
	where, args := kindFilter(kind)
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM accounts WHERE `+where, args...).Scan(&n)
	return n, err
}
