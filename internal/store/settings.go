package store

import (
	"context"
	"database/sql"
	"errors"
)

// Settings 是落库的运行配置。
type Settings struct {
	APIKey      string
	Proxy       string
	Domain      string
	Region      string
	DefaultJobs int
	DefaultWait int
	AddPassword bool
	AddMFA      bool
}

// DefaultSettings 返回首启默认。
func DefaultSettings() Settings {
	return Settings{
		Domain:      "icloud.com",
		Region:      "US",
		DefaultJobs: 1,
		DefaultWait: 5,
		AddPassword: true,
		AddMFA:      true,
	}
}

// GetSettings 读配置；无行时返回默认值。
func (s *Store) GetSettings(ctx context.Context) (Settings, error) {
	var st Settings
	var pw, mfa int
	err := s.db.QueryRowContext(ctx,
		`SELECT api_key, proxy, domain, region, default_jobs, default_wait, add_password, add_mfa
		 FROM settings WHERE id=1`).
		Scan(&st.APIKey, &st.Proxy, &st.Domain, &st.Region, &st.DefaultJobs, &st.DefaultWait, &pw, &mfa)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultSettings(), nil
	}
	if err != nil {
		return Settings{}, err
	}
	st.AddPassword = pw == 1
	st.AddMFA = mfa == 1
	return st, nil
}

// PutSettings 整行覆盖。
func (s *Store) PutSettings(ctx context.Context, st Settings) error {
	b := func(v bool) int {
		if v {
			return 1
		}
		return 0
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO settings(id, api_key, proxy, domain, region, default_jobs, default_wait,
			add_password, add_mfa, updated_at)
		 VALUES(1,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET
			api_key=excluded.api_key, proxy=excluded.proxy, domain=excluded.domain, region=excluded.region,
			default_jobs=excluded.default_jobs, default_wait=excluded.default_wait,
			add_password=excluded.add_password, add_mfa=excluded.add_mfa,
			updated_at=excluded.updated_at`,
		st.APIKey, st.Proxy, st.Domain, st.Region, st.DefaultJobs, st.DefaultWait,
		b(st.AddPassword), b(st.AddMFA), now())
	return err
}

// Account 是账号行。
type Account struct {
	ID          int64
	Email       string
	Password    string
	TotpSecret  string
	Name        string
	Birthdate   string
	Token       string
	Proxy       string
	IP          string
	HasPassword bool
	MFAState    string
	Attempts    int
	Error       string
	CreatedAt   int64
	UpdatedAt   int64
}

// InsertAccount 写入新账号，返回 id。
func (s *Store) InsertAccount(ctx context.Context, a Account) (int64, error) {
	ts := now()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO accounts(email, password, totp_secret, name, birthdate, token,
			proxy, ip, has_password, mfa_state, created_at, updated_at)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		a.Email, a.Password, a.TotpSecret, a.Name, a.Birthdate, a.Token,
		a.Proxy, a.IP, b2i(a.HasPassword), a.MFAState, ts, ts)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateToken 只改登录态。
func (s *Store) UpdateToken(ctx context.Context, id int64, token string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE accounts SET token=?, updated_at=? WHERE id=?`, token, now(), id)
	return affected(res, err)
}

// SetMFAState 改异步状态与原因。
func (s *Store) SetMFAState(ctx context.Context, id int64, state, note string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE accounts SET mfa_state=?, error=?, updated_at=? WHERE id=?`,
		state, note, now(), id)
	return affected(res, err)
}

// BumpAttempts 重试计数加一并返回新值。
func (s *Store) BumpAttempts(ctx context.Context, id int64) (int, error) {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE accounts SET attempts=attempts+1, updated_at=? WHERE id=?`, now(), id); err != nil {
		return 0, err
	}
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT attempts FROM accounts WHERE id=?`, id).Scan(&n)
	return n, err
}

// GetAccount 按 id 取一条。
func (s *Store) GetAccount(ctx context.Context, id int64) (Account, error) {
	var a Account
	var hp int
	err := s.db.QueryRowContext(ctx,
		`SELECT id,email,password,totp_secret,name,birthdate,token,proxy,ip,
			has_password,mfa_state,attempts,error,created_at,updated_at
		 FROM accounts WHERE id=?`, id).
		Scan(&a.ID, &a.Email, &a.Password, &a.TotpSecret, &a.Name, &a.Birthdate,
			&a.Token, &a.Proxy, &a.IP, &hp, &a.MFAState, &a.Attempts, &a.Error,
			&a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	a.HasPassword = hp == 1
	return a, err
}

func b2i(v bool) int {
	if v {
		return 1
	}
	return 0
}

func affected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
