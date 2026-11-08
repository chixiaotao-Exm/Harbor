package main

import "fmt"

// 版本信息由构建时注入：
//
//	go build -ldflags "-X main.Version=v1.2.3 -X main.Commit=$(git rev-parse --short HEAD) -X main.BuildTime=$(date -u +%FT%TZ)"
//
// 未注入时保持 dev，便于本地直接 run。
var (
	Version   = "dev"
	Commit    = "none"
	BuildTime = "unknown"
)

func versionLine() string {
	return fmt.Sprintf("harbor %s (%s)", Version, Commit)
}
