#!/bin/bash

# 在当前目录启动单节点 etcd 实例，用于本地开发测试

DIR="$(cd "$(dirname "$0")" && pwd)"
DATA_DIR="${DIR}/default.etcd"

exec etcd \
  --data-dir "${DATA_DIR}" \
  --listen-client-urls http://127.0.0.1:2379 \
  --advertise-client-urls http://127.0.0.1:2379 \
  --listen-peer-urls http://127.0.0.1:2380 \
  --initial-advertise-peer-urls http://127.0.0.1:2380 \
  --initial-cluster default=http://127.0.0.1:2380 \
  --name default
