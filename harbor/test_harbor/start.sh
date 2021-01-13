#!/bin/zsh
nohup go run ../../center/main/center.go localhost:8012 teacher > center2.log 2>&1 &

#group a
nohup go run ../../center/main/center.go localhost:8010 teacher > center1.log 2>&1 &
nohup go run ../../harbor/harbor.go localhost:8010@localhost:8012 1.255.1 localhost:9101 teacher > harbor1.log 2>&1 &

#group b
nohup go run ../../center/main/center.go localhost:8011 teacher > center3.log 2>&1 &
nohup go run ../../harbor/harbor.go localhost:8011@localhost:8012 2.255.1 localhost:9102 teacher > harbor2.log 2>&1 &