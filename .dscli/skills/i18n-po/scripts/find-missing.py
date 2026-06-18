#!/usr/bin/env python3
"""
find-missing.py — Find i18n.G() msgids missing from a .po file.

Usage:
  python3 find-missing.py                          # compare against zh_CN
  python3 find-missing.py --po path/to/file.po     # compare against specific .po

Output: JSON-escaped msgids (one per line) printed to stdout.
        Summary statistics printed to stderr.
"""

import re, os, sys, argparse, json
def extract_msgids(content):
    """Extract all i18n.G(...) argument strings from Go source."""
    msgids = set()
    for m in re.finditer(r'i18n\.G\(', content):
        pos = m.end()
        if pos >= len(content):
            continue
        ch = content[pos]
        if ch == '"':
            # Double-quoted Go string literal — need to unescape
            chars = []
            end = pos + 1
            while end < len(content):
                c = content[end]
                if c == '\\':
                    # Go escape sequence
                    if end + 1 >= len(content):
                        break
                    next_c = content[end + 1]
                    if next_c == 'n':
                        chars.append('\n')
                    elif next_c == 't':
                        chars.append('\t')
                    elif next_c == 'r':
                        chars.append('\r')
                    elif next_c == '\\':
                        chars.append('\\')
                    elif next_c == '"':
                        chars.append('"')
                    elif next_c == 'x' and end + 3 < len(content):
                        # \xNN hex escape
                        try:
                            chars.append(chr(int(content[end+2:end+4], 16)))
                            end += 2
                        except ValueError:
                            chars.append('\\')
                            chars.append(next_c)
                    else:
                        chars.append('\\')
                        chars.append(next_c)
                    end += 2
                elif c == '"':
                    break
                else:
                    chars.append(c)
                    end += 1
            msgid = ''.join(chars)
            msgids.add(msgid)
        elif ch == '`':
            # Backtick raw string literal — no escaping needed
            end = content.index('`', pos + 1)
            msgid = content[pos+1:end]
            msgids.add(msgid)
    return msgids

def unescape_po(s):
    """Unescape .po escape sequences: \\n -> newline, \\\\ -> backslash, \\\" -> quote."""
    s = s.replace('\\n', '\n')
    s = s.replace('\\\\', '\\')
    s = s.replace('\\"', '"')
    return s

def load_po_msgids(filepath):
    """Load all msgids from a .po file, unescaping \\n sequences."""
    with open(filepath) as f:
        content = f.read()
    msgids = set()
    lines = content.split('\n')
    i = 0
    while i < len(lines):
        line = lines[i]
        if line.startswith('msgid ""'):
            i += 1
            parts = []
            while i < len(lines) and lines[i].startswith('"'):
                part = lines[i].strip()[1:-1]
                parts.append(part)
                i += 1
            full = ''.join(parts)
            if full:
                msgids.add(unescape_po(full))
        elif line.startswith('msgid "'):
            text = line[7:-1]
            i += 1
            while i < len(lines) and lines[i].startswith('"'):
                text += lines[i].strip()[1:-1]
                i += 1
            if text:
                msgids.add(unescape_po(text))
        else:
            i += 1
    return msgids

def main():
    parser = argparse.ArgumentParser(description='Find missing i18n.G() msgids in .po file')
    parser.add_argument('--po', default=None,
                        help='Path to .po file (default: internal/i18n/locales/zh_CN/slingshot.po)')
    parser.add_argument('--source', default='.',
                        help='Source root directory (default: current dir)')
    args = parser.parse_args()

    repo = os.getcwd()
    po_path = args.po
    if po_path is None:
        po_path = os.path.join(repo, 'internal/i18n/locales/zh_CN/slingshot.po')
    if not os.path.exists(po_path):
        print(f"Error: .po file not found: {po_path}", file=sys.stderr)
        sys.exit(1)

    # Get all source msgids
    all_msgids = set()
    for root, dirs, files in os.walk(args.source):
        skip = False
        for seg in root.split(os.sep):
            if seg in ('vendor', 'testdata', '.git'):
                skip = True
                break
        if skip:
            continue
        for f in files:
            if f.endswith('.go'):
                fp = os.path.join(root, f)
                try:
                    with open(fp, errors='replace') as fh:
                        all_msgids.update(extract_msgids(fh.read()))
                except Exception as e:
                    print(f"Warning: {fp}: {e}", file=sys.stderr)

    po_msgids = load_po_msgids(po_path)

    # Find missing
    missing = sorted(all_msgids - po_msgids)
    print(f"Source msgids: {len(all_msgids)}", file=sys.stderr)
    print(f".po msgids:    {len(po_msgids)}", file=sys.stderr)
    print(f"Missing:       {len(missing)}", file=sys.stderr)

    for m in missing:
        # JSON-encode to preserve multi-line msgids on a single line
        print(json.dumps(m, ensure_ascii=False))

if __name__ == '__main__':
    main()
