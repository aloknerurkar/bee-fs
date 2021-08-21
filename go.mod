module github.com/aloknerurkar/bee-fs

go 1.15

require (
	github.com/billziss-gh/cgofuse v1.5.0
	github.com/briandowns/spinner v1.15.0
	github.com/cheynewallace/tabby v1.1.1
	github.com/dgraph-io/badger/v3 v3.2103.0
	github.com/ethersphere/bee v0.6.2-0.20210528134436-c0f60e24b2fa
	github.com/golang/gddo v0.0.0-20210115222349-20d68f94ee1f
	github.com/gorilla/mux v1.8.0
	github.com/gorilla/websocket v1.4.2
	github.com/ipfs/go-log/v2 v2.3.0
	github.com/mitchellh/go-homedir v1.1.0
	github.com/robfig/cron/v3 v3.0.0
	github.com/spf13/cobra v1.0.0
	go.etcd.io/bbolt v1.3.6
	go.uber.org/atomic v1.8.0
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
)

replace github.com/ethersphere/bee => ../bee
