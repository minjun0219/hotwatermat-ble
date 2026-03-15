#!/usr/bin/env bash
# hotwatermat-ble 설치 스크립트
# 사용법: curl -fsSL https://raw.githubusercontent.com/minjun0219/hotwatermat-ble/main/install.sh | bash
set -euo pipefail

REPO="minjun0219/hotwatermat-ble"
BINARY_NAME="hotwatermat-ble"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

# --- 색상 출력 ---
info()  { printf '\033[1;34m%s\033[0m\n' "$*"; }
ok()    { printf '\033[1;32m%s\033[0m\n' "$*"; }
warn()  { printf '\033[1;33m%s\033[0m\n' "$*"; }
error() { printf '\033[1;31m%s\033[0m\n' "$*" >&2; }

# --- OS 감지 ---
detect_os() {
  case "$(uname -s)" in
    Darwin) echo "darwin" ;;
    Linux)  echo "linux" ;;
    MINGW*|MSYS*|CYGWIN*)
      error "Windows는 직접 지원하지 않습니다."
      error "WSL(Windows Subsystem for Linux)에서 다시 실행해 주세요:"
      error "  wsl -- bash -c 'curl -fsSL https://raw.githubusercontent.com/$REPO/main/install.sh | bash'"
      exit 1
      ;;
    *)
      error "지원하지 않는 OS입니다: $(uname -s)"
      exit 1
      ;;
  esac
}

# --- 아키텍처 감지 ---
detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64)   echo "amd64" ;;
    arm64|aarch64)   echo "arm64" ;;
    *)
      error "지원하지 않는 아키텍처입니다: $(uname -m)"
      exit 1
      ;;
  esac
}

# --- 최신 릴리즈 태그 조회 ---
fetch_latest_version() {
  local url="https://api.github.com/repos/${REPO}/releases/latest"
  local tag

  if command -v curl &>/dev/null; then
    tag=$(curl -fsSL "$url" | grep '"tag_name"' | head -1 | sed -E 's/.*"tag_name":\s*"([^"]+)".*/\1/')
  elif command -v wget &>/dev/null; then
    tag=$(wget -qO- "$url" | grep '"tag_name"' | head -1 | sed -E 's/.*"tag_name":\s*"([^"]+)".*/\1/')
  else
    error "curl 또는 wget이 필요합니다."
    exit 1
  fi

  if [ -z "$tag" ]; then
    error "최신 릴리즈를 찾을 수 없습니다."
    exit 1
  fi

  echo "$tag"
}

# --- 다운로드 ---
download() {
  local url="$1" dest="$2"
  if command -v curl &>/dev/null; then
    curl -fsSL -o "$dest" "$url"
  else
    wget -qO "$dest" "$url"
  fi
}

# --- 메인 ---
main() {
  info "hotwatermat-ble 설치 스크립트"
  echo ""

  local os arch version archive url tmpdir

  os=$(detect_os)
  arch=$(detect_arch)
  info "시스템: ${os}/${arch}"

  info "최신 버전 확인 중..."
  version=$(fetch_latest_version)
  info "버전: ${version}"

  archive="${BINARY_NAME}_${version}_${os}_${arch}.tar.gz"
  url="https://github.com/${REPO}/releases/download/${version}/${archive}"

  tmpdir=$(mktemp -d)
  trap 'rm -rf "$tmpdir"' EXIT

  info "${archive} 다운로드 중..."
  download "$url" "${tmpdir}/${archive}"

  info "바이너리 추출 중..."
  tar xzf "${tmpdir}/${archive}" -C "$tmpdir" "$BINARY_NAME"

  mkdir -p "$INSTALL_DIR"
  mv "${tmpdir}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
  chmod +x "${INSTALL_DIR}/${BINARY_NAME}"

  # macOS: quarantine 속성 제거
  if [ "$os" = "darwin" ]; then
    xattr -d com.apple.quarantine "${INSTALL_DIR}/${BINARY_NAME}" 2>/dev/null || true
  fi

  ok ""
  ok "설치 완료: ${INSTALL_DIR}/${BINARY_NAME}"
  ok "버전: ${version}"

  # PATH 확인
  case ":${PATH}:" in
    *":${INSTALL_DIR}:"*) ;;
    *)
      echo ""
      warn "주의: ${INSTALL_DIR}이(가) PATH에 포함되어 있지 않습니다."
      warn "아래 명령어를 셸 설정 파일에 추가해 주세요:"
      echo ""
      echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
      echo ""
      ;;
  esac
}

main
