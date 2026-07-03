#!/usr/bin/env bash
# ParkPulse — image build va GHCR'ga push.
# Ishlatish:
#   ./build.sh          -> build qiladi va push qiladi
#   ./build.sh --local  -> faqat build (push qilmaydi, test uchun)
set -euo pipefail

IMAGE="ghcr.io/jamolovmn/parking-pulse:latest"
cd "$(dirname "$0")"   # skript qayerda bo'lsa, loyiha ildizi shu

echo "==> Build: $IMAGE"
docker build -t parkpulse:latest -t "$IMAGE" .

if [[ "${1:-}" == "--local" ]]; then
  echo "==> Faqat lokal build tayyor (push qilinmadi)."
  exit 0
fi

echo "==> GHCR'ga push..."
docker push "$IMAGE"
echo "==> Tayyor: $IMAGE"
echo "    ioEdge'da konteynerni re-pull qiling."
