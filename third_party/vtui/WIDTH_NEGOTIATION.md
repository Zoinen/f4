# Proposal: one source of truth for character widths

Status: **idea**, not scheduled, not started. Written down so it is not lost.
Nothing in vtui depends on this. `UNICODE_PLAN.md` stage 8 is the local,
unilateral version of the same problem, and it is what would actually get done
first.

## The problem

Every layer decides independently how wide a character is. The terminal has a
table. The application has a table. The C library has a third one, the
language runtime a fourth. They come from different Unicode versions and
different judgement calls about ambiguous width and emoji presentation. When
they disagree the text is drawn in the right cells by one party and read out
of the wrong cells by another, and the line shifts. Every full screen terminal
application has this bug and none of them can fix it alone, because the fix
requires agreement.

Two halves to a solution: agree on a **format** for a width table, and agree
on a **protocol** for telling a terminal which one to use.

## Part one: a declarative table format

A plain text, line oriented file. Machine writable, human diffable, trivially
parseable in any language, which matters more than compactness on disk.

    # unicode-width 1
    unicode-version: 15.1.0
    id: ucd-15.1-emoji-wide
    ambiguous: narrow
    emoji-presentation: wide

    0300..036F 0
    1100..115F 2
    200D       0
    FE0F       0

Rules: ranges are sorted and non overlapping; width is 0, 1 or 2; anything not
listed is 1; comments start with `#`. The header names the Unicode version and
carries an `id` by which the table can be referred to without transferring it.
The two judgement calls that cause most disagreement - ambiguous width and
emoji presentation - are named in the header rather than baked silently into
the ranges, so two tables that differ only in policy can be compared.

Lookup order, each layer overriding the one before:

1. `/usr/share/unicode-width/` - what the distribution ships.
2. `$XDG_CONFIG_HOME/unicode-width/` - the user's overrides, which is the
   point of the whole exercise: one place to fix a character everywhere.
3. Whatever the application was told to use explicitly.

An override file is the same format and contains only the ranges it changes.

## Part two: negotiating with the terminal

Having a shared file helps only when both parties read files from the same
machine. Over ssh they do not. So the application must be able to tell the
terminal which table to use, or at least find out which one it is using.

Four operations, and the ordering matters, because the cheap ones usually make
the expensive one unnecessary.

**Identify.** The application asks what table the terminal uses and gets back
an id and a hash. This is a few dozen bytes and answers the question most of
the time: if the id matches what the application has, there is nothing to do.

**Fetch.** The application asks for the terminal's table. Usually unnecessary
after Identify, but it is the input to a delta, and downloading is cheaper
than uploading on a typical link.

**Apply delta.** The application sends only the ranges where its table differs
from the terminal's. This is the operation that has to be compact, and it is
the one that benefits from the terminal's table being known first.

**Push and pop.** The applied table is a stack entry, so an application can
restore the previous state when it exits, and a crash does not leave the
terminal permanently reconfigured. A terminal may also expire the stack on
reset.

### On making the delta small

The observation that makes this cheap: a delta is a sorted list of
`(start, end, width)` triples, so almost every number in it is small relative
to the previous one, and the width is one of three values.

- Encode `start` as the gap from the previous range's end, not as an absolute
  code point. Gaps are small; absolute code points are up to six hex digits.
- Encode `end` as the length of the range, not as a position.
- Pack the width into two bits and carry it in the first byte of the length.
- Use a variable length encoding over a printable alphabet, since this has to
  survive a terminal escape sequence. Base64 without padding is the obvious
  choice and is already what other terminal protocols use for bulk data.

With that, a typical range costs two to four bytes instead of fourteen, and a
realistic delta between two tables built a Unicode version apart is a few
hundred bytes. A whole table is a few kilobytes, which is also acceptable, but
the delta makes it unnecessary in the common case.

Send it in chunks, because terminal input buffers are not large, and give each
chunk a sequence number so a truncated transfer is detected rather than
silently applied in part.

### When there is no answer

Terminals that do not implement this must be assumed to be the majority
forever. The application waits a short time for a reply, then falls back.

The fallback that actually works is measurement: draw a sentinel character,
ask for the cursor position with the standard report, and see how far it
moved. This is what existing width probing tools do. It costs one round trip
per character measured, so it is only viable for a handful of sentinels - one
per contested class, not one per code point. Pick a sentinel for each
disagreement that matters: an emoji with a presentation selector, an ambiguous
width character, a ZWJ sequence, a regional indicator pair. Four round trips
buy most of the benefit.

Cache the result against the terminal's identity so the probe happens once per
session rather than once per screen.

## Why this is worth writing down even if nobody builds it

The format alone is useful without the protocol: it gives a user one file to
edit instead of one per application, and it gives applications something to
generate their compiled tables from. The protocol only pays off with terminal
adoption, which is a long game and not vtui's to play alone. If any of this
gets built here, build the format first and the local override support with
it; the protocol can wait for someone to be interested.