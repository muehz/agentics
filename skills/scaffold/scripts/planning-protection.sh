#!/bin/sh
# Preview the Agentics planning protection block for a repository's root .gitignore.
#
# Usage: planning-protection.sh --parent <relative-parent> [--gitignore <existing-root-.gitignore>]
#
# Prints the full merged .gitignore on stdout and a limitation note on stderr.
# Writes no file. Exits 0 on success and 1 on a usage, parent, marker or
# encoding error. The only template is ../references/planning-protection.gitignore.
# Needs POSIX sh and the standard tools awk, cat, head, od, printf, sed, tail
# and tr.

# Every tool works on bytes: UTF-8 validity is checked below by byte value, and
# the file's own bytes are never re-encoded.
LC_ALL=C
export LC_ALL
# Every awk below runs with BINMODE=3: gawk on Windows otherwise drops the CR of
# a CRLF while reading. Other awks treat it as an unused variable.

BEGIN='# BEGIN AGENTICS PLANNING PROTECTION'
END='# END AGENTICS PLANNING PROTECTION'

fail() {
  printf '%s\n' "$1" >&2
  exit 1
}

usage() {
  fail 'Usage: planning-protection.sh --parent <relative-parent> [--gitignore <existing-root-.gitignore>]'
}

parent=
have_parent=0
gitignore=
have_gitignore=0
while [ "$#" -gt 0 ]; do
  [ "$#" -ge 2 ] || usage
  value=$2
  case $value in --*) usage ;; esac
  case $1 in
    --parent)
      [ "$have_parent" -eq 0 ] || usage
      parent=$value
      have_parent=1
      ;;
    --gitignore)
      [ "$have_gitignore" -eq 0 ] || usage
      gitignore=$value
      have_gitignore=1
      ;;
    *) usage ;;
  esac
  shift 2
done
[ "$have_parent" -eq 1 ] || usage

# One trailing slash is conventional and dropped; any other empty component is not.
case $parent in */) parent=${parent%/} ;; esac
# A line break in the parent is found by removing every CR and LF and seeing
# whether anything changed. Keep this out of the case patterns below: the
# command guard reads an expansion in a case pattern as one in command position
# and would deny running this script.
plain=$(printf '%s' "$parent" | tr -d '\r\n')
[ "$plain" = "$parent" ] || fail 'Invalid planning parent: use a nonempty repository-relative slash-separated path'
case $parent in
  '' | /* | */ | *//* | *\\* | *:*)
    fail 'Invalid planning parent: use a nonempty repository-relative slash-separated path'
    ;;
esac
# The dots are bracketed on purpose: the command guard reads a bare . pattern
# word as the source builtin and would deny running this script.
case $parent in
  [.] | [.][.] | [.]/* | [.][.]/* | */[.] | */[.][.] | */[.]/* | */[.][.]/*)
    fail 'Invalid planning parent: traversal components are forbidden'
    ;;
esac

# The template sits beside this script's directory. A Windows-style $0 is
# normalised so that the parent directory can be cut at the last slash.
case $0 in
  *\\*) self=$(printf '%s\n' "$0" | tr '\134' '/') ;;
  *) self=$0 ;;
esac
case $self in
  */*) dir=${self%/*} ;;
  *) dir=. ;;
esac
template="${dir:-/}/../references/planning-protection.gitignore"
[ -f "$template" ] && [ -r "$template" ] || fail "Cannot read the planning protection template: $template"

# Characters that .gitignore treats as pattern syntax get a backslash. The
# backslash itself cannot occur: a parent containing one was rejected above.
escaped=$(printf '%s\n' "$parent" | sed 's/[]*?[!# ]/\\&/g')

# emit_block <crlf>: the template with the parent filled in, one line ending
# per template line. The parent travels through the environment so that awk
# does not interpret its backslashes.
emit_block() {
  PARENT_TEXT=$escaped awk -v BINMODE=3 -v crlf="$1" '
    BEGIN {
      token = "{{PARENT}}"
      parent = ENVIRON["PARENT_TEXT"]
      eol = (crlf == 1) ? "\r\n" : "\n"
    }
    {
      line = $0
      if (substr(line, length(line), 1) == "\r") line = substr(line, 1, length(line) - 1)
      out = ""
      while ((i = index(line, token)) > 0) {
        out = out substr(line, 1, i - 1) parent
        line = substr(line, i + length(token))
      }
      printf "%s%s", out line, eol
    }
  ' < "$template"
}

crlf=0
status=none
begin_line=0
end_line=0
last=-1
if [ "$have_gitignore" -eq 1 ]; then
  [ -f "$gitignore" ] && [ -r "$gitignore" ] || fail "Cannot read the .gitignore file: $gitignore"

  # Strict UTF-8 as the WHATWG decoder defines it: no overlong forms, no
  # surrogates, nothing above U+10FFFF. The BOM (EF BB BF) is valid text and is
  # kept. The last byte is reported too, to tell an empty or unterminated file.
  verdict=$(od -An -v -tu1 < "$gitignore" | awk '
    BEGIN { last = -1 }
    {
      for (i = 1; i <= NF; i++) {
        b = $i + 0
        last = b
        if (need > 0) {
          if (b < lo || b > hi) bad = 1
          need--
          lo = 128
          hi = 191
        } else if (b < 128) {
        } else if (b >= 194 && b <= 223) { need = 1; lo = 128; hi = 191
        } else if (b == 224) { need = 2; lo = 160; hi = 191
        } else if (b == 237) { need = 2; lo = 128; hi = 159
        } else if (b >= 225 && b <= 239) { need = 2; lo = 128; hi = 191
        } else if (b == 240) { need = 3; lo = 144; hi = 191
        } else if (b >= 241 && b <= 243) { need = 3; lo = 128; hi = 191
        } else if (b == 244) { need = 3; lo = 128; hi = 143
        } else bad = 1
      }
    }
    END { printf "%d %d\n", !(bad || need > 0), last }
  ')
  # shellcheck disable=SC2086
  set -- $verdict
  [ "$1" -eq 1 ] || fail 'Invalid .gitignore encoding: expected UTF-8'
  last=$2

  # Lines end at LF; a CR before that LF belongs to the line ending. A final
  # line with no LF has no line ending, so its CR stays part of the text. The
  # file is CRLF when any line ends that way. Marker lines are the lines that
  # hold either marker text: there must be none, or exactly the BEGIN line
  # followed by the END line, each standing alone.
  ends_lf=0
  [ "$last" -eq 10 ] && ends_lf=1
  scan=$(awk -v BINMODE=3 -v endlf="$ends_lf" -v begin="$BEGIN" -v end="$END" '
    function scan_line(line, terminated) {
      lineno++
      if (terminated && substr(line, length(line), 1) == "\r") {
        crlf = 1
        line = substr(line, 1, length(line) - 1)
      }
      if (index(line, begin) > 0 || index(line, end) > 0) {
        markers++
        if (markers == 1) { first = line; first_at = lineno }
        if (markers == 2) { second = line; second_at = lineno }
      }
    }
    { if (have) scan_line(held, 1); held = $0; have = 1 }
    END {
      if (have) scan_line(held, endlf)
      state = "none"
      if (markers > 0) state = (markers == 2 && first == begin && second == end) ? "ok" : "bad"
      printf "%d %s %d %d\n", crlf + 0, state, first_at + 0, second_at + 0
    }
  ' < "$gitignore")
  # shellcheck disable=SC2086
  set -- $scan
  crlf=$1
  status=$2
  begin_line=$3
  end_line=$4
fi

[ "$status" != bad ] || fail 'Invalid planning protection markers: expected exactly one complete standalone block'

printf '%s\n' 'Limitation: existing ignored ancestors can prevent discovery; this preview does not unignore ancestors. Ignores cannot untrack files or prevent force-adds; native validation remains separate.' >&2

case $status in
  none)
    if [ "$have_gitignore" -eq 1 ]; then
      cat < "$gitignore"
      # Existing text that does not end in LF is terminated before the block.
      if [ "$last" -ge 0 ] && [ "$last" -ne 10 ]; then
        if [ "$crlf" -eq 1 ]; then printf '\r\n'; else printf '\n'; fi
      fi
    fi
    emit_block "$crlf"
    ;;
  ok)
    # Everything before the BEGIN line and after the END line is kept byte for byte.
    [ "$begin_line" -le 1 ] || head -n "$((begin_line - 1))" < "$gitignore"
    emit_block "$crlf"
    tail -n "+$((end_line + 1))" < "$gitignore"
    ;;
esac
exit 0
