#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Turns the result block of a lab run into a terminal picture (SVG) for the
README. The text is the lab's own output, unchanged; long lines are wrapped.

    bash scripts/lab-netns.sh | tee lab.txt
    python3 scripts/lab-svg.py < lab.txt > docs/images/lab.svg
"""
import html
import re
import sys

WRAP = 92
FONT, LINE, CHAR = 13, 19, 7.85  # px; CHAR is the width of one monospace character


def result_block(text):
    lines = text.splitlines()
    try:
        start = next(i for i, l in enumerate(lines) if l.startswith("== Result"))
    except StopIteration:
        sys.exit("no '== Result' block in the input")
    out = []
    for line in lines[start:]:
        if line.startswith(("logs and trace:", "exit ")):  # local paths stay out
            break
        out.append(line.rstrip())
    return out


def wrap(line):
    rows = []
    while len(line) > WRAP:
        cut = line.rfind(" ", 0, WRAP)
        cut = cut if cut > 20 else WRAP
        rows.append(line[:cut])
        line = "       " + line[cut:].lstrip()
    rows.append(line)
    return rows


def spans(row):
    """The row as SVG text, with "ok" in green and "FAILED" in red."""
    parts = re.split(r"\b(ok|FAILED)\b", row)
    out = []
    for i, p in enumerate(parts):
        if i % 2 == 1:
            color = "#3fb950" if p == "ok" else "#f85149"
            out.append('<tspan fill="%s" font-weight="bold">%s</tspan>' % (color, p))
        else:
            out.append(html.escape(p))
    return "".join(out)


def main():
    rows = ["$ bash scripts/lab-netns.sh", "..."]
    for line in result_block(sys.stdin.read()):
        rows += wrap(line)
    width = int(2 * 18 + CHAR * WRAP)
    height = 34 + 14 + LINE * len(rows) + 10
    svg = [
        '<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">' % (width, height, width, height),
        '<rect width="100%" height="100%" rx="8" fill="#0d1117"/>',
        '<rect width="100%" height="28" rx="8" fill="#161b22"/>',
        '<rect y="20" width="100%" height="8" fill="#161b22"/>',
        '<circle cx="16" cy="14" r="5" fill="#ff5f56"/><circle cx="32" cy="14" r="5" fill="#ffbd2e"/>'
        '<circle cx="48" cy="14" r="5" fill="#27c93f"/>',
        '<text x="%d" y="18" fill="#8b949e" font-family="ui-sans-serif, Helvetica, Arial, sans-serif" '
        'font-size="12" text-anchor="middle">AXLink lab: two stations, simulated channel</text>' % (width // 2),
        '<g font-family="ui-monospace, SFMono-Regular, Menlo, Consolas, \'DejaVu Sans Mono\', monospace" '
        'font-size="%d" fill="#c9d1d9" xml:space="preserve">' % FONT,
    ]
    for i, row in enumerate(rows):
        fill = ' fill="#8b949e"' if i < 2 else ""
        svg.append('<text x="18" y="%d"%s>%s</text>' % (34 + 14 + LINE * i, fill, spans(row)))
    svg += ["</g>", "</svg>"]
    sys.stdout.write("\n".join(svg) + "\n")


if __name__ == "__main__":
    main()
