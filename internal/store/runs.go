package store

import "context"

// OpenRun 记一条运行开始。
func (s *Store) OpenRun(ctx context.Context, total int) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO runs(started_at, total) VALUES(?, ?)`, now(), total)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// CloseRun 收尾一条运行。
func (s *Store) CloseRun(ctx context.Context, id int64, ok, fail int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE runs SET ended_at=?, ok=?, fail=? WHERE id=?`, now(), ok, fail, id)
	return err
}
