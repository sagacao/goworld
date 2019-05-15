# GoWorld
对goworld定制化的一些修改
具体可以参考
https://github.com/xiaonanln/goworld



Installing GoWorld:

go get -d github.com/sagacao/goworld
// go get -u github.com/golang/dep/cmd/dep
cd $GOPATH/src/github.com/sagacao/goworld
// dep ensure
go get ./cmd/...

Build:
--> dispathcer/gate:
------> cd $GOPATH/src/github.com/sagacao/goworld
		go build github.com/sagacao/goworld/components/dispatcher
		go build github.com/sagacao/goworld/components/gate
		goworld build components
--> heros
------> cd $GOPATH/src/heros
		go build
		goworld build heros

Start:
./components/dispatcher/dispatcher -d -dispid 1 -configfile ./config/goworld.ini
./components/gate/gate -d -gid 1 -configfile ./config/goworld.ini
./heros -d -gid 1 -configfile ./config/goworld.ini

goworld start heros

Stop:
kill -15 [pid]

goworld stop heros