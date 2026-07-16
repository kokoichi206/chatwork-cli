#!/bin/sh
#
# Description:
#   リリースタグを origin/main の先頭に打って push する。
#   push された v* タグが GitHub Actions の release workflow を起動し、
#   goreleaser が GitHub Releases にバイナリを公開する。
#   push の前に release workflow と同じ検証(tools/releaseguard)をローカルで実行し、
#   失敗した場合は作成したローカルタグを削除して終了する。
#
# Usage:
#   scripts/release.sh vMAJOR.MINOR.PATCH
#   scripts/release.sh -h | --help
#
# Example:
#   scripts/release.sh v0.1.0
set -eu

repo="kokoichi206/chatwork-cli"

fail() {
	echo "error: $1" >&2
	exit 1
}

usage() {
	cat <<'EOF'
Usage: scripts/release.sh vMAJOR.MINOR.PATCH

Tags the tip of origin/main and pushes the tag. The tag push triggers
the release workflow, which publishes binaries to GitHub Releases.
The tag is validated locally (tools/releaseguard) before the push.

Example:
  scripts/release.sh v0.1.0
EOF
}

case "${1:-}" in
-h | --help)
	usage
	exit 0
	;;
esac

[ $# -eq 1 ] || {
	usage >&2
	exit 2
}
version="$1"

command -v go >/dev/null 2>&1 || fail "go is required"

cd "$(git rev-parse --show-toplevel)"

if git rev-parse -q --verify "refs/tags/${version}" >/dev/null; then
	fail "tag ${version} already exists"
fi

git fetch origin

# タグはローカルの checkout 状態ではなく、常に fetch 直後の origin/main を指す。
git tag -a "$version" -m "$version" origin/main

# push 前に release workflow と同じ検証を通す。ここで落ちたタグは push させない。
if ! go run ./tools/releaseguard --tag "$version" --base origin/main; then
	git tag -d "$version" >/dev/null
	fail "validation failed; removed local tag ${version}"
fi

echo "tag ${version} -> $(git log -1 --format='%h %s' "${version}^{commit}")"
printf 'push tag %s to origin? [y/N] ' "$version"
read -r answer || answer=""
case "$answer" in
[yY] | [yY][eE][sS]) ;;
*)
	git tag -d "$version" >/dev/null
	fail "aborted; removed local tag ${version}"
	;;
esac

git push origin "$version"
echo "pushed ${version}; release workflow: https://github.com/${repo}/actions/workflows/release.yml"
