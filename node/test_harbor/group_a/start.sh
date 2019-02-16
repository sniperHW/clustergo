#!/bin/sh
#nohup go run ../../../center/center.go localhost:8010 sanguo > center.log 2>&1 &
#nohup go run ../../../harbor/harbor.go localhost:8010@localhost:8012 1.255.1 localhost:9101 sanguo > harbor.log 2>&1 &
go run node1.go localhost:8010