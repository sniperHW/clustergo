#!/bin/sh
nohup go run ../../center/center.go localhost:8010 sanguo > center1.log 2>&1 &
nohup go run ../../harbor/harbor.go localhost:8010@localhost:8012 1.255.1 localhost:9101 sanguo > harbor1.log 2>&1 &
nohup go run ../../center/center.go localhost:8012 sanguo > center2.log 2>&1 &

nohup go run ../../center/center.go localhost:8011 sanguo > center3.log 2>&1 &
nohup go run ../../harbor/harbor.go localhost:8011@localhost:8012 2.255.1 localhost:9102 sanguo > harbor2.log 2>&1 &