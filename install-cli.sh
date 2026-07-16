#!/usr/bin/env bash
# ParkPulse CLI — hostga global buyruq o'rnatadi.
# O'rnatgandan keyin VPS'da xuddi `claude` kabi shunchaki `pulse` (yoki `parkpulse`)
# deb yozib, ishlab turgan konteyner ichidagi AI agentga ulanasiz.
#
# Ishlatish (server/VPS'da):
#   sudo ./install-cli.sh
set -euo pipefail

# Image nomining bir qismi — build.sh dagi IMAGE bilan mos.
IMAGE_MATCH="parking-pulse"
BIN_DIR="/usr/local/bin"

# Wrapper skript: ishlab turgan ParkPulse konteynerini topib, ichida pulse-cli'ni ochadi.
read -r -d '' WRAPPER <<'EOF' || true
#!/usr/bin/env bash
# ParkPulse CLI wrapper (install-cli.sh tomonidan yaratilgan).
set -euo pipefail
IMAGE_MATCH="parking-pulse"

# docker'ga ruxsat: guruhda bo'lmasa sudo'ga o'tamiz — `pulse` sudosiz ishlaydi.
if docker info >/dev/null 2>&1; then
  DK="docker"
elif command -v sudo >/dev/null 2>&1; then
  DK="sudo docker"
else
  echo "docker'ga ruxsat yo'q. Foydalanuvchini docker guruhiga qo'shing:" >&2
  echo "  sudo usermod -aG docker \$USER   (keyin qayta kiring)" >&2
  exit 1
fi

# 1) Image nomi bo'yicha ishlab turgan konteynerni top.
CID="$($DK ps --format '{{.ID}} {{.Image}}' | awk -v m="$IMAGE_MATCH" '$2 ~ m {print $1; exit}')"
# 2) Topilmasa — konteyner nomi bo'yicha urinib ko'r.
if [ -z "${CID:-}" ]; then
  CID="$($DK ps --format '{{.ID}} {{.Names}}' | awk '$2 ~ /pulse/ {print $1; exit}')"
fi
if [ -z "${CID:-}" ]; then
  echo "ParkPulse konteyneri ishlamayapti. Avval konteynerni ishga tushiring (docker ps bo'sh)." >&2
  exit 1
fi
# -it faqat terminal bo'lsa; quvur/skriptda -it bermaymiz.
if [ -t 0 ] && [ -t 1 ]; then TTY="-it"; else TTY="-i"; fi
# Parol o'rnatilgan bo'lsa, konteynerga uzatamiz (host env avtomatik o'tmaydi).
PW_ARG=()
if [ -n "${PULSE_PASSWORD:-}" ]; then PW_ARG=(-e "PULSE_PASSWORD=$PULSE_PASSWORD"); fi
exec $DK exec $TTY "${PW_ARG[@]}" "$CID" pulse-cli "$@"
EOF

install_one() {
  local name="$1"
  printf '%s\n' "$WRAPPER" > "$BIN_DIR/$name"
  chmod +x "$BIN_DIR/$name"
  echo "  ✓ $BIN_DIR/$name"
}

if [ ! -w "$BIN_DIR" ]; then
  echo "Yozish huquqi yo'q: $BIN_DIR — 'sudo ./install-cli.sh' bilan ishga tushiring." >&2
  exit 1
fi

echo "==> ParkPulse CLI buyruqlari o'rnatilmoqda ($IMAGE_MATCH):"
install_one pulse
install_one parkpulse
echo "==> Tayyor. Endi shunchaki 'pulse' yoki 'parkpulse' deb yozing."
echo "    Parol o'rnatgan bo'lsangiz: PULSE_PASSWORD='parol' pulse"
