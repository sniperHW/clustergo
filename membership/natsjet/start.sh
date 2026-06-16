#!/bin/bash
# 启动单节点 nats-server，开启 JetStream，store-dir=当前目录

DIR="$(cd "$(dirname "$0")" && pwd)"
JSDIR="${DIR}/js"
mkdir -p "${JSDIR}"

cat > "${DIR}/nats-server.conf" <<EOF
jetstream {
  store_dir: "${JSDIR}"
}
EOF

exec nats-server \
  -c "${DIR}/nats-server.conf" \
  -a 127.0.0.1 \
  -p 4222
