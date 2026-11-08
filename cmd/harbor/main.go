// Command harbor 是服务入口。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/chixi/harbor/internal/api"
	"github.com/chixi/harbor/internal/config"
	"github.com/chixi/harbor/internal/events"
	"github.com/chixi/harbor/internal/mfa"
	"github.com/chixi/harbor/internal/netx"
	"github.com/chixi/harbor/internal/runner"
	"github.com/chixi/harbor/internal/store"
	"github.com/chixi/harbor/web"
)

func main() {
	var (
		addr       = flag.String("addr", "127.0.0.1:1321", "监听地址")
		dataDir    = flag.String("data", "./data", "数据目录")
		webDir     = flag.String("web", "", "前端产物目录，留空用内嵌产物")
		check      = flag.Bool("check", false, "只做自检并退出")
		showVer    = flag.Bool("version", false, "打印版本并退出")
		schemaDump = flag.Bool("schema", false, "打印建表语句并退出")
	)
	flag.Parse()

	if *showVer {
		fmt.Printf("harbor %s (commit %s, built %s)\n", Version, Commit, BuildTime)
		return
	}

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	log.Info("版本", "ver", Version, "commit", Commit)

	if err := os.MkdirAll(*dataDir, 0o750); err != nil {
		log.Error("建数据目录失败", "err", err)
		os.Exit(1)
	}

	cfg, err := config.Load(filepath.Join(*dataDir, "config.toml.json"))
	if err != nil {
		log.Error("读配置失败", "err", err)
		os.Exit(1)
	}

	dbPath := filepath.Join(*dataDir, "harbor.db")
	st, err := store.Open(dbPath)
	if err != nil {
		log.Error("打开数据库失败", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	if *schemaDump {
		dumpSchema(st)
		return
	}

	hub := events.NewHub()
	rn := &runner.Runner{Store: st, Hub: hub, Marker: netx.NewMarker(func(m string) { log.Info(m) })}
	eng := &mfa.Engine{Store: st, Hub: hub, Marker: netx.NewMarker(func(m string) { log.Info(m) })}
	eng.New = func(ctx context.Context, proxy string) (mfa.Worker, error) {
		return mfa.NewLiveWorker(netx.NewMarker(func(m string) { log.Info(m) }), proxy)(ctx, proxy)
	}
	// 开户成功后自动投递到异步队列。
	rn.Enqueue = func(ctx context.Context, ids []int64, proxy string) (int, error) {
		return eng.Start(ctx, ids, proxy)
	}
	srv := &api.Server{Store: st, Cfg: cfg, Log: log, WebDir: *webDir, Hub: hub, Runner: rn, MFA: eng,
		Version: Version, Commit: Commit, BuildTime: BuildTime}
	if *webDir == "" {
		if embedded, err := webassets.FS(); err == nil {
			srv.WebFS = embedded
		}
	}
	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           api.New(srv),
		ReadHeaderTimeout: 10 * time.Second,
	}

	if *check {
		log.Info("自检通过", "db", dbPath, "addr", *addr)
		return
	}

	go func() {
		log.Info("已启动", "addr", *addr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("监听失败", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Info("收到退出信号，收尾中")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
}

func dumpSchema(st *store.Store) {
	rows, err := st.DB().Query(
		`SELECT sql FROM sqlite_master WHERE sql IS NOT NULL ORDER BY name`)
	if err != nil {
		os.Stdout.WriteString("dump: " + err.Error() + "\n")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		os.Stdout.WriteString(s + ";\n")
	}
}
