#!/bin/sh
# ParkPulse konteyner entrypoint'i.
#
# Image ishga tushishi bilan host'ga `pulse` va `parkpulse` buyruqlarini AVTO
# o'rnatadi — xuddi VPS'da `claude` kabi, hech qanday qo'lda qadamsiz. Buning
# uchun deploy'da host'ning bin papkasi mount qilingan bo'lishi kifoya:
#     -v /usr/local/bin:/host/bin
# Mount bo'lmasa — jimgina o'tkazib yuboriladi, server baribir normal ishlaydi.
#
# O'rnatilgan wrapper host'da ishlaydi: ishlab turgan ParkPulse konteynerini
# topib, ichidagi `pulse-cli`'ni `docker exec` bilan ochadi.
set -e

HOST_BIN="${PULSE_HOST_BIN:-/host/bin}"

install_host_cli() {
  dir="$1"
  [ -d "$dir" ] || return 0
  # Haqiqiy yozish sinovi: `[ -w ]` root + read-only mount uchun yolg'on true berishi
  # mumkin, shuning uchun vaqtinchalik fayl bilan tekshiramiz.
  if ! ( : > "$dir/.pulse-write-test" ) 2>/dev/null; then
    echo "[pulse-cli] $dir yozib bo'lmadi (read-only?) — host buyruqlari o'tkazib yuborildi."
    return 0
  fi
  rm -f "$dir/.pulse-write-test"
  for name in pulse parkpulse; do
    cat > "$dir/$name" <<'WRAP'
#!/usr/bin/env bash
# ParkPulse CLI wrapper — ParkPulse image entrypoint'i tomonidan avto yaratilgan.
# Ishlab turgan ParkPulse konteynerini topib, ichidagi AI agent CLI'ni ochadi.
set -euo pipefail
IMAGE_MATCH="parking-pulse"

CID="$(docker ps --format '{{.ID}} {{.Image}}' | awk -v m="$IMAGE_MATCH" '$2 ~ m {print $1; exit}')"
if [ -z "${CID:-}" ]; then
  CID="$(docker ps --format '{{.ID}} {{.Names}}' | awk '$2 ~ /pulse/ {print $1; exit}')"
fi
if [ -z "${CID:-}" ]; then
  echo "ParkPulse konteyneri ishlamayapti (docker ps bo'sh)." >&2
  exit 1
fi

if [ -t 0 ] && [ -t 1 ]; then TTY="-it"; else TTY="-i"; fi
PW_ARG=()
if [ -n "${PULSE_PASSWORD:-}" ]; then PW_ARG=(-e "PULSE_PASSWORD=$PULSE_PASSWORD"); fi
exec docker exec $TTY "${PW_ARG[@]}" "$CID" pulse-cli "$@"
WRAP
    chmod +x "$dir/$name"
  done
  echo "[pulse-cli] host buyruqlari o'rnatildi: $dir/pulse, $dir/parkpulse"
}

# O'rnatish hech qachon serverni to'xtatmasin (xato bo'lsa ham davom etamiz).
install_host_cli "$HOST_BIN" || true

# Serverni ishga tushiramiz (asosiy jarayon).
exec parkpulse "$@"
