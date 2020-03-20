#!/bin/zsh
#start center
nohup go run ../center/main/center.go localhost:6060 sanguo > center.log 2>&1 &

#start gameserver
nohup go run gameserver/main.go localhost:6060 sanguo > gameserver.log 2>&1 &

#start gateserver
nohup go run gateserver/main.go localhost:6060 sanguo > gateserver.log 2>&1 &
