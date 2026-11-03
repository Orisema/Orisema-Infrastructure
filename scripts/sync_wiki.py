#!/usr/bin/env python3
"""Publish repository Markdown to the GitHub Wiki, preserving its existing Home."""
from pathlib import Path
import re
import sys

root = Path(__file__).resolve().parents[1]
wiki = Path(sys.argv[1])
sources = sorted(root.glob("*.md")) + sorted((root / "docs").rglob("*.md"))
sources = [p for p in sources if p.name != "README.md"]
# Prefix docs pages to avoid collisions with root-level Markdown files.
def page_name(path):
    relative = path.relative_to(root)
    return ("Docs-" if relative.parts[0] == "docs" else "") + "-".join(relative.with_suffix("").parts)

mapping = {p.resolve(): page_name(p) for p in sources}
pattern = re.compile(r"(?<!!)\[([^\]]+)\]\(([^)]+)\)")

def rewrite(match, source):
    label, target = match.groups()
    path, sep, anchor = target.partition("#")
    if not path or "://" in path or path.startswith(("mailto:", "/")):
        return match.group(0)
    dest = (source.parent / path).resolve()
    if dest in mapping:
        return f"[{label}]({mapping[dest]}{('#' + anchor) if sep else ''})"
    # Non-Markdown files remain linked to the source repository.
    if dest.is_file() and dest.is_relative_to(root):
        rel = dest.relative_to(root).as_posix()
        return f"[{label}](https://github.com/Orisema/Orisema-Infrastructure/blob/main/{rel}{('#' + anchor) if sep else ''})"
    return match.group(0)

for source, name in mapping.items():
    content = source.read_text(encoding="utf-8")
    content = pattern.sub(lambda match: rewrite(match, source), content)
    (wiki / f"{name}.md").write_text(content, encoding="utf-8")

# Generated sidebar, without overwriting the manually created Home page.
lines = ["**Orisema Infrastructure**", "", "- [Home](Home)", ""]
for source in sources:
    name = mapping[source.resolve()]
    title = source.stem.replace("-", " ").replace("_", " ").title()
    lines.append(f"- [{title}]({name})")
(wiki / "_Sidebar.md").write_text("\n".join(lines) + "\n", encoding="utf-8")
print(f"Prepared {len(sources)} pages; Home.md preserved.")
