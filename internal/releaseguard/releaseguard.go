// Package releaseguard はリリースタグの不変条件を CI で検証する。
// 不変条件: リリースタグは main の先頭コミットからのみ切れる。バージョンは単調増加する。
package releaseguard

import (
	"fmt"
	"strconv"
	"strings"
)

// Version は vMAJOR.MINOR.PATCH 形式のタグからパースした安定版バージョン。
type Version struct {
	Major int
	Minor int
	Patch int
}

// ParseTag は安定版リリースタグをパースする。
// vMAJOR.MINOR.PATCH のみ許可し、pre-release 接尾辞や leading zero は拒否する。
func ParseTag(tag string) (Version, error) {
	rest, ok := strings.CutPrefix(tag, "v")
	if !ok {
		return Version{}, fmt.Errorf("release tag %q must start with v", tag)
	}

	parts := strings.Split(rest, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("release tag %q must use vMAJOR.MINOR.PATCH", tag)
	}

	major, err := parsePart(tag, parts[0])
	if err != nil {
		return Version{}, err
	}
	minor, err := parsePart(tag, parts[1])
	if err != nil {
		return Version{}, err
	}
	patch, err := parsePart(tag, parts[2])
	if err != nil {
		return Version{}, err
	}

	return Version{Major: major, Minor: minor, Patch: patch}, nil
}

// Compare は v が other より古ければ -1、等しければ 0、新しければ 1 を返す。
func (v Version) Compare(other Version) int {
	switch {
	case v.Major != other.Major:
		return compareInt(v.Major, other.Major)
	case v.Minor != other.Minor:
		return compareInt(v.Minor, other.Minor)
	default:
		return compareInt(v.Patch, other.Patch)
	}
}

// Validate はタグが base commit を指し、既存のすべてのリリースタグより新しいことを検証する。
// パースできない既存タグはリリースタグとして扱わず無視する。
func Validate(tag string, tagCommit string, baseCommit string, existingTags []string) error {
	current, err := ParseTag(tag)
	if err != nil {
		return err
	}

	if tagCommit != baseCommit {
		return fmt.Errorf("release tag %s points at %s, but origin/main is %s", tag, tagCommit, baseCommit)
	}

	for _, existingTag := range existingTags {
		if existingTag == tag {
			continue
		}

		existing, err := ParseTag(existingTag)
		if err != nil {
			continue
		}
		if current.Compare(existing) <= 0 {
			return fmt.Errorf("release tag %s must be newer than existing tag %s", tag, existingTag)
		}
	}

	return nil
}

func parsePart(tag string, part string) (int, error) {
	if part == "" {
		return 0, fmt.Errorf("release tag %q must use vMAJOR.MINOR.PATCH", tag)
	}
	if len(part) > 1 && part[0] == '0' {
		return 0, fmt.Errorf("release tag %q must not contain leading zeroes", tag)
	}

	value, err := strconv.Atoi(part)
	if err != nil {
		return 0, fmt.Errorf("release tag %q must use numeric version parts", tag)
	}
	return value, nil
}

func compareInt(a int, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
