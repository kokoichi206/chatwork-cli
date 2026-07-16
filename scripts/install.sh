#!/bin/sh
set -eu

repo="kokoichi206/chatwork-cli"
install_dir="${CW_INSTALL_DIR:-${HOME}/.local/bin}"

fail() {
	echo "error: $1" >&2
	exit 1
}

os=$(uname -s)
case "$os" in
Darwin) os="darwin" ;;
Linux) os="linux" ;;
*) fail "unsupported OS: ${os} (only darwin and linux are supported)" ;;
esac

arch=$(uname -m)
case "$arch" in
x86_64 | amd64) arch="amd64" ;;
arm64 | aarch64) arch="arm64" ;;
*) fail "unsupported architecture: ${arch} (only amd64 and arm64 are supported)" ;;
esac

command -v curl >/dev/null 2>&1 || fail "curl is required"

tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT INT TERM

tag=$(curl -fsSL -H "Accept: application/vnd.github+json" \
	"https://api.github.com/repos/${repo}/releases/latest" |
	sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' |
	head -n 1)
[ -n "$tag" ] || fail "could not determine the latest release tag"

version="${tag#v}"
# asset 名は .goreleaser.yaml の name_template と揃えること。
asset="chatwork-cli_${version}_${os}_${arch}.tar.gz"

curl -fsSL -o "${tmp_dir}/${asset}" \
	"https://github.com/${repo}/releases/download/${tag}/${asset}"
tar -xzf "${tmp_dir}/${asset}" -C "$tmp_dir" cw
chmod +x "${tmp_dir}/cw"
mkdir -p "$install_dir"
mv "${tmp_dir}/cw" "${install_dir}/cw"

echo "cw ${tag} installed to ${install_dir}/cw"
