# f4-ini-upsert - write declared ini keys into a live file without taking it over.
#
# Usage: f4-ini-upsert TARGET DESIRED
#
# Every "key = value" line of every [Section] in DESIRED is written into
# TARGET: the line is replaced where TARGET already has that key in that
# section, and appended to the section (or to the file, for a section TARGET
# does not have) otherwise. Everything else in TARGET - other sections,
# comments, blank lines, the order of what is already there - is preserved
# exactly. A missing or empty TARGET is created from DESIRED.
#
# The merge rule is the one f4's own ini reader uses to recognize a setting:
# a key line is any line holding "=", split at the first one, both sides
# trimmed; a section header is a line between "[" and "]".
#
# f4 keeps settings and state in the same files (settings.ini, hotkeys.ini)
# and rewrites them at runtime, so Home Manager cannot own those files as
# store links: f4's atomic writes would replace the link and the next
# `home-manager switch` would collide with the replacement. This merge is
# what lets Home Manager assert the keys it declares and leave the rest of
# the live file alone.

set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "usage: f4-ini-upsert TARGET DESIRED" >&2
  exit 2
fi

target=$1
desired=$2

if [[ ! -f $desired ]]; then
  echo "f4-ini-upsert: DESIRED not found: $desired" >&2
  exit 2
fi

mkdir -p "$(dirname "$target")"

# A missing or empty TARGET is merged as an empty file - every declared
# section is new and the result is DESIRED's own text. Reading /dev/null
# keeps DESIRED's validation on this path too.
target_input=$target
if [[ ! -s $target ]]; then
  target_input=/dev/null
fi

tmp=$target.tmp.$$
trap 'rm -f "$tmp"' EXIT
umask 077

awk '
function trim(s) {
  sub(/^[ \t\r]+/, "", s)
  sub(/[ \t\r]+$/, "", s)
  return s
}

FILENAME == ARGV[1] {
  t = trim($0)
  if (t ~ /^\[.*\]$/) {
    sec = substr(t, 2, length(t) - 2)
    have_section = 1
    if (!((sec) in seen_sec)) {
      seen_sec[sec] = 1
      sec_order[++sec_count] = sec
    }
    next
  }
  if (index(t, "=") > 0) {
    if (!have_section) {
      print "f4-ini-upsert: " FILENAME ": key outside any section: " t > "/dev/stderr"
      failed = 1
      exit 2
    }
    key = trim(substr(t, 1, index(t, "=") - 1))
    if (!((sec SUBSEP key) in desired_line)) {
      key_order[sec, ++key_count[sec]] = key
    }
    desired_line[sec, key] = t
    next
  }
  next
}

FILENAME == ARGV[2] {
  target_line[++target_count] = $0
  next
}

END {
  if (failed) {
    exit 2
  }

  # The section each target line belongs to is the section header at or
  # before it; a header line opens its own section. For every section that
  # TARGET has, anchor is its last non-blank line - where missing keys are
  # appended - and any_line proves the section exists at all.
  for (i = 1; i <= target_count; i++) {
    t = trim(target_line[i])
    if (t ~ /^\[.*\]$/) {
      target_sec[i] = substr(t, 2, length(t) - 2)
    } else {
      target_sec[i] = (i > 1 ? target_sec[i - 1] : "")
    }
    any_line[target_sec[i]] = i
    if (t != "") {
      anchor[target_sec[i]] = i
    }
  }

  for (s = 1; s <= sec_count; s++) {
    sec = sec_order[s]
    for (j = 1; j <= key_count[sec]; j++) {
      key = key_order[sec, j]
      replaced = 0
      for (i = 1; i <= target_count; i++) {
        if (target_sec[i] != sec) {
          continue
        }
        t = trim(target_line[i])
        if (t ~ /^\[/) {
          continue
        }
        p = index(t, "=")
        if (p == 0) {
          continue
        }
        # Every occurrence is replaced: f4 keeps the last duplicate it reads,
        # so a leftover one could win over the declared value.
        if (trim(substr(t, 1, p - 1)) == key) {
          target_line[i] = desired_line[sec, key]
          replaced = 1
        }
      }
      if (replaced) {
        continue
      }
      if ((sec) in anchor) {
        insert_after[anchor[sec]] = insert_after[anchor[sec]] desired_line[sec, key] "\n"
      } else if ((sec) in any_line) {
        insert_after[any_line[sec]] = insert_after[any_line[sec]] desired_line[sec, key] "\n"
      } else if ((sec) in new_lines) {
        new_lines[sec] = new_lines[sec] desired_line[sec, key] "\n"
      } else {
        new_lines[sec] = "[" sec "]\n" desired_line[sec, key] "\n"
      }
    }
  }

  for (i = 1; i <= target_count; i++) {
    print target_line[i]
    if (i in insert_after) {
      printf "%s", insert_after[i]
    }
  }
  wrote_something = (target_count > 0)
  for (s = 1; s <= sec_count; s++) {
    sec = sec_order[s]
    if (!((sec) in new_lines)) {
      continue
    }
    if (wrote_something) {
      print ""
    }
    printf "%s", new_lines[sec]
    wrote_something = 1
  }
}
' "$desired" "$target_input" > "$tmp"

mv "$tmp" "$target"
trap - EXIT
