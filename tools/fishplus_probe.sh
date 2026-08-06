#!/bin/sh
# f4 FISH+ remote host probe, version 2.
#
# Run this on a machine you would like to browse with f4 over FISH+ and
# paste the whole output into the FISH+ compatibility issue. It only reads:
# nothing is created, changed or removed, and nothing outside the current
# directory is touched.
#
#   sh fishplus_probe.sh 2>&1 | tee fishplus-probe.txt
#
# The output contains a listing of the current directory and of /, so run it
# somewhere you do not mind sharing.

echo "=== probe version"
echo 2
echo "=== system"
uname -a 2>&1
echo "=== shell"
ls -l /bin/sh 2>&1
echo "shell: ${0}"
echo "version: ${BASH_VERSION:-}${KSH_VERSION:-}${ZSH_VERSION:-}"
echo "=== tools"
for t in sh dash ash bash ksh busybox base64 openssl uuencode stat find ls dd \
         head tail cat readlink du grep sed awk wc expr tr cut printf stty od \
         xxd sha256sum md5sum cksum mktemp; do
	p=`command -v $t 2>/dev/null` || p=MISSING
	echo "$t: $p"
done
echo "=== base64 decode"
# Each decoder gets a labelled line: the trailing newline in the input
# matters, openssl silently decodes nothing without it.
printf 'base64 -d: '; printf 'aGk=\n' | base64 -d 2>&1; echo
printf 'base64 -D: '; printf 'aGk=\n' | base64 -D 2>&1; echo
printf 'base64 --decode: '; printf 'aGk=\n' | base64 --decode 2>&1; echo
printf 'openssl: '; printf 'aGk=\n' | openssl base64 -A -d 2>&1; echo
printf 'openssl, no newline: '; printf 'aGk=' | openssl base64 -A -d 2>&1; echo
echo "=== base64 encode wrapping"
printf '0123456789012345678901234567890123456789012345678901234567890123456789' | base64 2>&1
printf 'x' | base64 -w0 2>&1
echo
echo "=== find -printf"
find -H . -mindepth 0 -maxdepth 0 -printf '%y %Y %s %T@ %A@ %C@ %m %U %G %f\n' 2>&1
echo "=== find -maxdepth"
find . -maxdepth 1 -name . 2>&1 | head -2
echo "=== gnu stat"
stat -c '%f %s %Y %X %Z %u %g %n' . 2>&1
echo "=== bsd stat"
stat -f '%p %z %m %a %c %u %g %N' . 2>&1
echo "=== ls -l"
ls -l -A . 2>&1 | head -5
echo "=== ls -A support"
ls -A . >/dev/null 2>&1 && echo "ls -A: ok" || echo "ls -A: unsupported"
echo "=== ls date format"
LC_ALL=C ls -l -a / 2>&1 | head -6
echo "=== ls numeric ids"
LC_ALL=C ls -l -n -a / 2>&1 | head -4
echo "=== ls full timestamps"
echo "-- gnu --time-style"
LC_ALL=C ls -l --time-style=+%s -d / 2>&1 | head -2
LC_ALL=C ls -l --time-style=full-iso -d / 2>&1 | head -2
echo "-- gnu --full-time"
LC_ALL=C ls -l --full-time -d / 2>&1 | head -2
echo "-- bsd -T"
LC_ALL=C ls -lT / 2>&1 | head -2
echo "-- bsd -D"
LC_ALL=C ls -l -D '%s' / 2>&1 | head -2
echo "=== ls symlink display"
LC_ALL=C ls -l -d /bin 2>&1
echo "=== ls color"
ls --color=never -d . 2>&1
echo "=== ls -b"
ls -b -d . 2>&1
echo "=== dd plain"
dd if=/dev/zero of=/dev/null bs=16 skip=1 count=1 2>&1
echo "=== dd iflag"
dd if=/dev/zero of=/dev/null bs=1 count=1 iflag=skip_bytes,count_bytes 2>&1
echo "=== dd oflag/conv"
dd if=/dev/zero of=/dev/null bs=1 count=1 conv=notrunc oflag=seek_bytes 2>&1
echo "=== head -c"
printf 12345 | { head -c 2 2>&1; printf '|'; cat; }
echo
echo "=== tail -c"
printf 12345 | tail -c +3 2>&1
echo
printf 12345 | tail -c 2 2>&1
echo
echo "=== read builtin"
printf 'a b  c \n' | ( IFS= read -r line; printf '[%s]\n' "$line" )
printf 'back\\slash\n' | ( IFS= read -r line; printf '[%s]\n' "$line" )
echo "=== read long line"
awk 'BEGIN{s="";for(i=0;i<9000;i++)s=s "x";print s}' 2>/dev/null | ( IFS= read -r line; printf 'got %s chars\n' "${#line}" )
echo "=== shell arithmetic"
echo $((4294967296 + 1)) 2>&1
echo $((9007199254740992 + 1)) 2>&1
echo "=== printf escapes"
printf 'A\101\x41|\n' 2>&1
echo "=== tty"
[ -t 0 ] && echo "stdin: tty" || echo "stdin: not a tty"
stty -a 2>&1 | head -2
echo "=== locale"
locale 2>&1 | head -3
echo "=== done"
