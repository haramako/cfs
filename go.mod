module github.com/haramako/cfs

go 1.12

require (
	github.com/AdRoll/goamz v0.0.0-20170825154802-2731d20f46f4
	github.com/mitchellh/go-homedir v1.1.0
	github.com/natefinch/atomic v0.0.0-20150920032501-a62ce929ffcc
	github.com/pkg/errors v0.8.1
	github.com/urfave/cli v1.22.5
	golang.org/x/net v0.0.0-20190813141303-74dc4d7220e7
	golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45
	golang.org/x/sync v0.0.0-20190423024810-112230192c58
	golang.org/x/text v0.3.2
	google.golang.org/api v0.8.0
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	local.package/cfs v0.0.0-00010101000000-000000000000
)

replace local.package/cfs => ./
