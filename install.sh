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

  # 체크섬 검증
  info "체크섬 검증 중..."
  download "https://github.com/${REPO}/releases/download/${version}/checksums.txt" "${tmpdir}/checksums.txt" 2>/dev/null || true
  if [ -f "${tmpdir}/checksums.txt" ]; then
    local expected actual
    expected=$(grep "${archive}" "${tmpdir}/checksums.txt" | awk '{print $1}')
    if [ -n "$expected" ]; then
      if command -v sha256sum &>/dev/null; then
        actual=$(sha256sum "${tmpdir}/${archive}" | awk '{print $1}')
      elif command -v shasum &>/dev/null; then
        actual=$(shasum -a 256 "${tmpdir}/${archive}" | awk '{print $1}')
      fi
      if [ -n "$actual" ] && [ "$expected" != "$actual" ]; then
        error "체크섬 불일치! 다운로드가 손상되었을 수 있습니다."
        exit 1
      fi
      ok "체크섬 확인 완료 ✓"
    fi
  fi

  info "바이너리 추출 중..."
  tar xzf "${tmpdir}/${archive}" -C "$tmpdir" --strip-components=0
  # tar.gz 내부 ./ 접두사 대응
  if [ -f "${tmpdir}/./${BINARY_NAME}" ]; then
    mv "${tmpdir}/./${BINARY_NAME}" "${tmpdir}/${BINARY_NAME}"
  fi

  mkdir -p "$INSTALL_DIR"

  # 기존 버전 덮어쓰기 확인
  if [ -f "${INSTALL_DIR}/${BINARY_NAME}" ] && [ "${FORCE:-0}" != "1" ]; then
    local existing_ver
    existing_ver=$("${INSTALL_DIR}/${BINARY_NAME}" --version 2>/dev/null || echo "unknown")
    warn "기존 설치 발견: ${INSTALL_DIR}/${BINARY_NAME} (${existing_ver})"
    warn "${version}(으)로 덮어쓰시겠습니까? [y/N] "
    if [ -t 0 ]; then
      read -r answer
      if [ "$answer" != "y" ] && [ "$answer" != "Y" ]; then
        info "설치를 취소했습니다."
        exit 0
      fi
    else
      warn "비대화형 환경 — FORCE=1로 덮어쓰기 가능. 설치를 취소합니다."
      exit 0
    fi
  fi

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
