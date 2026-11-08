package store

import (
	"context"
	"fmt"
	"strings"
)

// ListAccounts 按页签分页。kind 为 mfa（已绑）或 plain（未绑）。
func (s *Store) ListAccounts(ctx context.Context, kind string, page, size int, q string) ([]Account, int, error) {
	where, args := kindFilter(kind)
	if strings.TrimSpace(q) != "" {
		where += " AND email LIKE ?"
		args = append(args, "%"+strings.TrimSpace(q)+"%")
	}
	if page < 1 {
		page = 1
	}
	switch {
	case size < 1:
		size = 20
	case size > 100:
		size = 100
	}

	var total int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM accounts WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	sqlText := fmt.Sprintf(
		`SELECT id,email,password,totp_secret,name,birthdate,proxy,ip,
			has_password,mfa_state,attempts,error,created_at
		 FROM accounts WHERE %s ORDER BY id DESC LIMIT ? OFFSET ?`, where)
	pageArgs := append(append([]any{}, args...), size, (page-1)*size)
	rows, err := s.db.QueryContext(ctx, sqlText, pageArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]Account, 0, size)
	for rows.Next() {
		var a Account
		var hp int
		if err := rows.Scan(&a.ID, &a.Email, &a.Password, &a.TotpSecret, &a.Name,
			&a.Birthdate, &a.Proxy, &a.IP, &hp, &a.MFAState, &a.Attempts,
			&a.Error, &a.CreatedAt); err != nil {
			return nil, 0, err
		}
		a.HasPassword = hp == 1
		out = append(out, a)
	}
	return out, total, rows.Err()
}

// kindFilter 给出页签条件，保证两页签不串。
func kindFilter(kind string) (string, []any) {
	if kind == "mfa" {
		return "totp_secret <> ''", nil
	}
	return "totp_secret = ''", nil
}

// ExportLines 按 id 列表或整页签导出，返回 txt 行。
func (s *Store) ExportLines(ctx context.Context, kind string, ids []int64) ([]string, error) {
	where, args := kindFilter(kind)
	if len(ids) > 0 {
		ph := make([]string, len(ids))
		for i, id := range ids {
			ph[i] = "?"
			args = append(args, id)
		}
		where += " AND id IN (" + strings.Join(ph, ",") + ")"
	}
	rs, err := s.db.QueryContext(ctx,
		`SELECT email,password,totp_secret FROM accounts WHERE `+where+` ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	defer rs.Close()

	lines := make([]string, 0, len(ids)+8)
	for rs.Next() {
		var email, pw, secret string
		if err := rs.Scan(&email, &pw, &secret); err != nil {
			return nil, err
		}
		if kind == "mfa" {
			lines = append(lines, email+"----"+pw+"----"+secret)
		} else {
			lines = append(lines, email+"----"+pw)
		}
	}
	return lines, rs.Err()
}

// AllIDs 返回某页签全部 id，供全选使用。
func (s *Store) AllIDs(ctx context.Context, kind, q string) ([]int64, error) {
	where, args := kindFilter(kind)
	if strings.TrimSpace(q) != "" {
		where += " AND email LIKE ?"
		args = append(args, "%"+strings.TrimSpace(q)+"%")
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM accounts WHERE `+where+` ORDER BY id DESC`, args...)
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
