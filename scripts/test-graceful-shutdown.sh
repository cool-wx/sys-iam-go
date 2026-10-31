#!/usr/bin/env bash
# 真实二进制 SIGTERM 排空验证（ENG-06 双重证明之二：真实进程证明）
# 入口：make verify-shutdown
#
# 流程：独立 mysql 容器（随机端口）→ DDL 引导 → 临时 config.yaml → make build →
#   TZ=UTC 启动 bin/sys-iam-go → /healthz 200 → kill -TERM → 退出码 0（≤20s）→
#   stdout 日志含 shutdown 完成行 → 清理。任一步失败非 0 退出并打印失败步骤。
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONTAINER_NAME="sys-iam-go-shutdown-test"
TMP_DIR="$(mktemp -d /tmp/sys-iam-go-shutdown.XXXXXX)"
APP_PID=""
WATCHDOG=""

fail() {
  echo "[FAIL] $1" >&2
  exit 1
}

cleanup() {
  if [ -n "$APP_PID" ] && kill -0 "$APP_PID" 2>/dev/null; then
    kill -KILL "$APP_PID" 2>/dev/null || true
  fi
  [ -n "$WATCHDOG" ] && kill "$WATCHDOG" 2>/dev/null || true
  docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

# 前置依赖
command -v docker >/dev/null 2>&1 || fail "前置: docker 不可用"
command -v curl >/dev/null 2>&1 || fail "前置: curl 不可用"
[ -f "$REPO_ROOT/scripts/ddl/sys_user.sql" ] || fail "前置: scripts/ddl/sys_user.sql 缺失"

# pick_free_port 用 /dev/tcp 探测本机空闲端口（connect 失败 = 空闲）
pick_free_port() {
  local port
  for _ in $(seq 1 20); do
    port=$(( RANDOM % 20000 + 30000 ))
    if ! (exec 3<>"/dev/tcp/127.0.0.1/$port") 2>/dev/null; then
      echo "$port"
      return 0
    fi
  done
  return 1
}

# ---- Step 1: 镜像选择（离线自适应，与 internal/testutil 同模式：
#      IAM_TEST_MYSQL_IMAGE env > 本地 mysql:8.4.x 缓存探测 > mysql:8.4 兜底）----
IMG="${IAM_TEST_MYSQL_IMAGE:-}"
if [ -z "$IMG" ]; then
  IMG="$(docker images --format '{{.Repository}}:{{.Tag}}' \
    | grep -E '^mysql:8\.4' | grep -v '<none>' | sort | tail -1 || true)"
fi
[ -n "$IMG" ] || IMG="mysql:8.4"
echo "[OK] Step 1 镜像: $IMG"

# ---- Step 2: 起独立容器（宿主端口由 docker 随机分配）----
docker run -d --rm --name "$CONTAINER_NAME" \
  -e MYSQL_DATABASE=sys_iam -e MYSQL_ROOT_PASSWORD=root123 \
  -p 127.0.0.1::3306 "$IMG" >/dev/null \
  || fail "Step 2: mysql 容器启动失败"
HOST_PORT="$(docker port "$CONTAINER_NAME" 3306 | head -1 | awk -F: '{print $NF}')"
[ -n "$HOST_PORT" ] || fail "Step 2: 容器端口解析失败"
echo "[OK] Step 2 容器已启动 127.0.0.1:$HOST_PORT"

# ---- Step 3: 等待 MySQL 就绪 + 应用 DDL ----
READY=0
for _ in $(seq 1 60); do
  if docker exec "$CONTAINER_NAME" mysql -uroot -proot123 \
    -e "USE sys_iam; SELECT 1;" >/dev/null 2>&1; then
    READY=1
    break
  fi
  sleep 1
done
[ "$READY" = 1 ] || fail "Step 3: MySQL 60s 内未就绪"
docker exec -i "$CONTAINER_NAME" mysql -uroot -proot123 sys_iam \
  < "$REPO_ROOT/scripts/ddl/sys_user.sql" \
  || fail "Step 3: DDL 应用失败"
echo "[OK] Step 3 MySQL 就绪 + DDL 已应用"

# ---- Step 4: 临时 config.yaml + 构建 ----
APP_PORT="$(pick_free_port)" || fail "Step 4: 无法找到空闲端口"
cat > "$TMP_DIR/config.yaml" <<EOF
server:
  port: $APP_PORT
mysql:
  dsn: "root:root123@tcp(127.0.0.1:$HOST_PORT)/sys_iam?charset=utf8mb4&parseTime=True&loc=Asia%2FShanghai"
  max_open_conns: 5
  max_idle_conns: 2
  conn_max_lifetime: 30m
snowflake:
  node: 1
log:
  level: info
EOF
( cd "$REPO_ROOT" && make build >/dev/null ) || fail "Step 4: make build 失败"
echo "[OK] Step 4 构建完成 + 临时配置 $TMP_DIR/config.yaml (app port=$APP_PORT)"

# ---- Step 5: TZ=UTC 启动真实二进制 → /healthz 200 ----
TZ=UTC "$REPO_ROOT/bin/sys-iam-go" -config "$TMP_DIR/config.yaml" \
  > "$TMP_DIR/stdout.log" 2>&1 &
APP_PID=$!
HEALTHZ_OK=0
for _ in $(seq 1 50); do
  if ! kill -0 "$APP_PID" 2>/dev/null; then
    echo "--- stdout.log ---" >&2
    cat "$TMP_DIR/stdout.log" >&2
    fail "Step 5: 服务进程提前退出"
  fi
  CODE="$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$APP_PORT/healthz" || true)"
  if [ "$CODE" = "200" ]; then
    HEALTHZ_OK=1
    break
  fi
  sleep 0.2
done
[ "$HEALTHZ_OK" = 1 ] || fail "Step 5: /healthz 10s 内未达 200"
echo "[OK] /healthz 200 (服务已就绪)"

# ---- Step 6: SIGTERM → 退出码 0 且 20s 内完成 ----
kill -TERM "$APP_PID" || fail "Step 6: 发送 SIGTERM 失败"
TERM_START=$(date +%s)
( sleep 20 && kill -KILL "$APP_PID" 2>/dev/null ) &
WATCHDOG=$!
set +e
wait "$APP_PID"
APP_RC=$?
set -e
kill "$WATCHDOG" 2>/dev/null && wait "$WATCHDOG" 2>/dev/null || true
WATCHDOG=""
TERM_END=$(date +%s)
ELAPSED=$((TERM_END - TERM_START))

if [ "$APP_RC" -ne 0 ]; then
  echo "--- stdout.log ---" >&2
  cat "$TMP_DIR/stdout.log" >&2
  fail "Step 6: SIGTERM 后退出码 $APP_RC（期望 0，耗时 ${ELAPSED}s）"
fi
if [ "$ELAPSED" -ge 20 ]; then
  fail "Step 6: SIGTERM 排空耗时 ${ELAPSED}s ≥ 20s 预算（watchdog 兜底已触发）"
fi
echo "[OK] SIGTERM graceful exit code 0 (elapsed ${ELAPSED}s)"

# ---- Step 7: stdout 日志含 shutdown 完成记录行（D-08：stdout→journald 同源）----
if ! grep -q "shutdown complete" "$TMP_DIR/stdout.log"; then
  echo "--- stdout.log ---" >&2
  cat "$TMP_DIR/stdout.log" >&2
  fail "Step 7: stdout 日志缺少 shutdown complete 记录行"
fi
echo "[OK] shutdown complete log line present"

echo "GRACEFUL SHUTDOWN VERIFIED (exit=$APP_RC elapsed=${ELAPSED}s log=shutdown-complete)"
