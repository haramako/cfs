module github.com/haramako/cfs

go 1.12

require (
	github.com/AdRoll/goamz v0.0.0-20170825154802-2731d20f46f4
	github.com/go-yaml/yaml v2.1.0+incompatible
	github.com/natefinch/atomic v0.0.0-20200526193002-18c0533a5b09
	github.com/pkg/errors v0.9.1
	github.com/urfave/cli v1.22.5
	golang.org/x/net v0.0.0-20201209123823-ac852fbbde11
	golang.org/x/oauth2 v0.0.0-20201208152858-08078c50e5b5
	golang.org/x/sync v0.0.0-20201020160332-67f06af15bc9
	golang.org/x/text v0.3.4
	google.golang.org/api v0.36.0
	local.package/cfs v0.0.0-00010101000000-000000000000
)

replace local.package/cfs => ./
