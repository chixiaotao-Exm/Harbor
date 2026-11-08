package store

import "context"

// PendingMFAIDs 返回待绑双因子的账号。
func (s *Store) PendingMFAIDs(ctx context.Context) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM accounts WHERE totp_secret='' ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// FailedRow 是失败页行。
type FailedRow struct {
	ID     int64  `json:"id"`
	Email  string `json:"email"`
	Reason string `json:"reason"`
	Times  int    `json:"times"`
}

// FailedMFA 列出失败页。
func (s *Store) FailedMFA(ctx context.Context) ([]FailedRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, email, error, attempts FROM accounts
		 WHERE mfa_state='fail' ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]FailedRow, 0, 8)
	for rows.Next() {
		var f FailedRow
		if err := rows.Scan(&f.ID, &f.Email, &f.Reason, &f.Times); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
