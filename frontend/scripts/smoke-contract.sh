#!/usr/bin/env bash
# 契约冒烟：真后端直证前端即将消费的核心契约——
# ① healthz 200；
# ② 登录失败（不存在的用户名 + 错误密码，无需真实凭证）→ 信封 "code":10501
#    （pkg/errcode CodeBadCredentials + gin JSON 紧凑信封直证）；
# ③ 无 token GET 受保护路由 → 信封 "code":10100（认证中间件 401 契约直证）。
# 纯 curl+grep，不依赖 jq。任一步不符 exit 1。
# 后端实例：8080 已有 sys-iam-go 在跑则复用；否则 go run ./cmd/server 后台启动、
# 日志重定向 /tmp/sys-iam-mug-smoke.log，脚本收尾 kill 仅清理自启实例。
set -euo pipefail

BASE="http://localhost:8080"
LOG="/tmp/sys-iam-mug-smoke.log"
SRV_PID=""

# 脚本位于 frontend/scripts/——切仓库根目录，go run ./cmd/server 路径恒成立
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

cleanup() {
  if [ -n "$SRV_PID" ] && kill -0 "$SRV_PID" 2>/dev/null; then
    pkill -P "$SRV_PID" 2>/dev/null || true
    kill "$SRV_PID" 2>/dev/null || true
    wait "$SRV_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

# ① 复用运行中实例，否则后台自启（sleep 4 首等 + 至多 10s 轮询就绪）。
# -config configs/config.yaml：仓库布局的配置路径（部署形态是二进制旁 config.yaml，
# 默认值 ./config.yaml 在仓库根不存在，缺省启动会落到内置默认 DSN 的 sys_iam 库）
if curl -sf "$BASE/healthz" >/dev/null 2>&1; then
  echo "[smoke] 复用已运行实例 $BASE"
else
  echo "[smoke] 自启 go run ./cmd/server -config configs/config.yaml（日志 $LOG）"
  go run ./cmd/server -config configs/config.yaml >"$LOG" 2>&1 &
  SRV_PID=$!
  sleep 4
  ready=0
  for _ in 1 2 3 4 5 6 7 8 9 10; do
    if curl -sf "$BASE/healthz" >/dev/null 2>&1; then
      ready=1
      break
    fi
    sleep 1
  done
  [ "$ready" = "1" ] || { echo "[smoke] FAIL: 后端 15s 内未就绪（见 $LOG）"; exit 1; }
fi

# ② healthz 200
curl -sf "$BASE/healthz" >/dev/null || { echo "[smoke] FAIL: healthz 未返回 200"; exit 1; }
echo "[smoke] PASS: healthz 200"

# ③ 登录失败 → 10501 信封（紧凑 JSON：grep 字面 "code":10501）
RESP=$(curl -s -X POST -H 'Content-Type: application/json' \
  -d '{"username":"__smoke_nonexistent__","password":"wrong-password"}' \
  "$BASE/api/v1/auth/login")
echo "$RESP" | grep -q '"code":10501' \
  || { echo "[smoke] FAIL: 登录失败响应不含 \"code\":10501 → $RESP"; exit 1; }
echo "[smoke] PASS: 登录失败返回 10501 信封"

# ④ 无 token 访问受保护路由 → 10100 信封（HTTP 401，curl 不加 -f 以读取响应体）
RESP=$(curl -s "$BASE/api/v1/users")
echo "$RESP" | grep -q '"code":10100' \
  || { echo "[smoke] FAIL: 无 token 响应不含 \"code\":10100 → $RESP"; exit 1; }
echo "[smoke] PASS: 无 token 访问返回 10100 信封"

echo "[smoke] 全部通过：healthz 200 / 登录失败 10501 / 无 token 10100"
