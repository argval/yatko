"""Train the release-asset filename classifier and export it for backend/assetml.

Reads harvester JSONL, derives os/arch/libc labels from inspected file headers,
fits one char n-gram logistic regression per head, prints grouped-CV metrics,
and writes model.json plus a Go parity fixture.

Usage:
    uv run --project ml python ml/train.py \
        --data ml/data/harvest.jsonl --out backend/assetml/model.json

Flags:
    --data          harvester JSONL file (default ml/data/harvest.jsonl)
    --out           exported model path (default backend/assetml/model.json)
    --parity        parity fixture path (default: <out dir>/testdata/parity.json)
    --max-features  vocabulary cap for the n-gram vectorizer (default 30000)
    --cs            comma-separated C grid to tune (default 0.3,1,3,10)
"""

from __future__ import annotations

import argparse
import json
import random
from pathlib import Path
from typing import Any

import numpy as np
from sklearn.feature_extraction.text import CountVectorizer
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import accuracy_score, confusion_matrix, f1_score
from sklearn.model_selection import GroupKFold

NGRAM = (2, 5)
HEADS = ("os", "arch", "libc")
PRUNE = 1e-4
UNSEEN = [
    "foo-1.2.3-aarch64-apple-darwin.tar.gz",
    "foo-1.2.3-x86_64-unknown-linux-musl.tar.gz",
    "foo-1.2.3-x86_64-pc-windows-msvc.zip",
    "Foo_Setup_2.0.0_arm64.exe",
    "foo_2.0.0_amd64.deb",
    "foo-2.0.0.x86_64.rpm",
    "Foo-2.0.0-universal.dmg",
    "foo  bar\t\tbaz-linux-armv7.tar.gz",
    "föö-ünïcode-darwin-arm64.zip",
    "x",
]

Row = dict[str, Any]
Labels = dict[str, str | None]


def load_rows(path: Path) -> list[Row]:
    """Read JSONL rows, skipping a trailing partial line, deduplicated on (repo, name)."""
    seen: set[tuple[str, str]] = set()
    rows: list[Row] = []
    for line in path.read_text().splitlines():
        try:
            row = json.loads(line)
        except json.JSONDecodeError:
            continue  # the harvester may still be writing the last line
        key = (row["repo"], row["name"])
        if key not in seen:
            seen.add(key)
            rows.append(row)
    return rows


# Yatko serves these OSes and CPUs; every other real value becomes "other", so
# the model still learns that e.g. a riscv64 or FreeBSD file is not a match.
SERVED_OS = {"linux", "macos", "windows", "android", "ios"}
SERVED_ARCH = {"amd64", "arm64", "arm", "386"}
# Harvester spellings of served values (it keeps raw codes it cannot map).
ARCH_ALIASES = {
    "i686": "386",
    "linux-i686": "386",
    "linux-amd64": "amd64",
    "linux-armhf": "arm",
}


def normalize_target(t: dict[str, str]) -> dict[str, str]:
    """Fold raw inspector values into the served classes plus "other"."""
    platform, arch, libc = t.get("platform", ""), t.get("arch", ""), t.get("libc", "")
    if platform == "musl":  # a musl loader identifies Linux, not an OS of its own
        platform, libc = "linux", libc or "musl"
    if platform and platform not in SERVED_OS:
        platform = "other"
    arch = ARCH_ALIASES.get(arch, arch)
    if arch and arch not in SERVED_ARCH:
        arch = "other"
    return {"platform": platform, "arch": arch, "libc": libc}


def derive_labels(row: Row) -> Labels:
    """Map one harvester row to os/arch/libc labels; None means masked for that head."""
    labels: Labels = dict.fromkeys(HEADS)
    targets = [normalize_target(t) for t in row.get("targets") or []]
    if row.get("status") != "inspected" or not targets or row.get("conflicts"):
        return labels
    platforms = {t["platform"] for t in targets} - {""}
    if len(platforms) != 1:
        return labels
    os_ = platforms.pop()
    labels["os"] = os_
    mine = [t for t in targets if t["platform"] == os_]
    archs = {t["arch"] for t in mine} - {""}
    # Windows installers (NSIS, Inno) wrap an x64/arm64 app in a 32-bit x86 stub,
    # so a 386 header on an .exe says nothing about the app: mask it.
    # ponytail: also drops the few real 32-bit .exe apps; fine at today's data size.
    if (
        os_ == "windows"
        and archs == {"386"}
        and str(row.get("name", "")).lower().endswith(".exe")
    ):
        archs = set()
    if os_ == "macos" and archs == {"amd64", "arm64"}:
        labels["arch"] = "universal"
    elif len(archs) == 1:
        labels["arch"] = archs.pop()
    if os_ == "linux":
        libcs = {t["libc"] for t in mine} - {""}
        if len(libcs) == 1:
            labels["libc"] = libcs.pop()
    return labels


def make_vectorizer(max_features: int) -> CountVectorizer:
    """Char n-gram presence features; Go reproduces this exactly in assetml."""
    return CountVectorizer(
        analyzer="char",
        ngram_range=NGRAM,
        lowercase=True,
        binary=True,
        min_df=2,
        max_features=max_features,
    )


def head_data(
    rows: list[Row], labels: list[Labels], head: str
) -> tuple[list[str], list[str], list[str]]:
    """Return (names, labels, repo groups) for the rows where this head has a label."""
    keep = [(r, lab[head]) for r, lab in zip(rows, labels) if lab[head] is not None]
    return (
        [r["name"] for r, _ in keep],
        [y for _, y in keep],
        [r["repo"] for r, _ in keep],
    )


def cv_report(
    names: list[str],
    y: list[str],
    groups: list[str],
    cs: list[float],
    max_features: int,
) -> float:
    """Tune C with GroupKFold by repo, print metrics for the best C, and return it."""
    n_splits = min(5, len(set(groups)))
    if n_splits < 2 or len(set(y)) < 2:
        print("  too little data for CV; using C=1")
        return 1.0
    y_arr, best = np.array(y), None
    for c in cs:
        pred = np.empty_like(y_arr)
        for tr, te in GroupKFold(n_splits).split(names, y_arr, groups):
            tr_y = y_arr[tr]
            if len(set(tr_y)) == 1:  # fold saw one class only; predict it
                pred[te] = tr_y[0]
                continue
            vec = make_vectorizer(max_features)
            x_tr = vec.fit_transform([names[i] for i in tr])
            clf = LogisticRegression(C=c, max_iter=5000).fit(x_tr, tr_y)
            pred[te] = clf.predict(vec.transform([names[i] for i in te]))
        acc = accuracy_score(y_arr, pred)
        f1 = f1_score(y_arr, pred, average="macro", zero_division=0)
        print(f"  C={c:<5} acc={acc:.3f} macroF1={f1:.3f}")
        if best is None or f1 > best[1]:
            best = (c, f1, pred)
    c, _, pred = best
    classes = sorted(set(y))
    print(f"  best C={c}; confusion (rows=true, cols=pred) {classes}")
    for cls, line in zip(classes, confusion_matrix(y_arr, pred, labels=classes)):
        print(f"    {cls:>10} {line.tolist()}")
    return c


def fit_head(
    names: list[str], y: list[str], c: float, max_features: int
) -> tuple[CountVectorizer, LogisticRegression]:
    """Fit the final vectorizer and classifier for one head on all its rows."""
    vec = make_vectorizer(max_features)
    clf = LogisticRegression(C=c, max_iter=5000).fit(vec.fit_transform(names), y)
    return vec, clf


def export_head(vec: CountVectorizer, clf: LogisticRegression) -> dict[str, Any]:
    """Convert one head to the JSON contract: softmax rows, pruned, rounded weights."""
    coef, intercept = clf.coef_, clf.intercept_
    if coef.shape[0] == 1:  # binary: softmax over [0, z] equals sigmoid(z)
        coef = np.vstack([np.zeros_like(coef), coef])
        intercept = np.array([0.0, intercept[0]])
    weights = {
        gram: [float(f"{w:.7g}") for w in coef[:, j]]
        for gram, j in sorted(vec.vocabulary_.items())
        if np.abs(coef[:, j]).max() >= PRUNE
    }
    return {
        "classes": [str(c) for c in clf.classes_],
        "intercept": [float(f"{b:.7g}") for b in intercept],
        "weights": weights,
    }


def predict_exported(
    head: dict[str, Any], vec: CountVectorizer, name: str
) -> dict[str, float]:
    """Score a name with the exported (pruned, rounded) weights, as Go will."""
    z = np.array(head["intercept"], dtype=float)
    analyze = vec.build_analyzer()
    for gram in set(analyze(name)):
        if gram in head["weights"]:
            z += np.array(head["weights"][gram])
    p = np.exp(z - z.max())
    p /= p.sum()
    return {c: float(v) for c, v in zip(head["classes"], p)}


def parity_names(rows: list[Row], k: int = 50) -> list[str]:
    """Pick a stable sample of real filenames plus a few unseen naming styles."""
    names = sorted({r["name"] for r in rows})
    return random.Random(0).sample(names, min(k, len(names))) + UNSEEN


def main() -> None:
    ap = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawTextHelpFormatter
    )
    ap.add_argument("--data", type=Path, default=Path("ml/data/harvest.jsonl"))
    ap.add_argument("--out", type=Path, default=Path("backend/assetml/model.json"))
    ap.add_argument("--parity", type=Path)
    ap.add_argument("--max-features", type=int, default=30000)
    ap.add_argument("--cs", default="0.3,1,3,10")
    args = ap.parse_args()
    cs = [float(c) for c in args.cs.split(",")]

    rows = load_rows(args.data)
    labels = [derive_labels(r) for r in rows]
    trained = sum(any(lab[h] for h in HEADS) for lab in labels)
    print(f"{len(rows)} unique rows, {trained} with at least one label")

    model: dict[str, Any] = {
        "version": 1,
        "ngram_min": NGRAM[0],
        "ngram_max": NGRAM[1],
        "trained_rows": trained,
        "heads": {},
    }
    fitted: dict[str, tuple[CountVectorizer, LogisticRegression]] = {}
    for head in HEADS:
        names, y, groups = head_data(rows, labels, head)
        counts = {c: y.count(c) for c in sorted(set(y))}
        print(f"\n[{head}] n={len(y)} repos={len(set(groups))} classes={counts}")
        if len(counts) < 2:
            raise SystemExit(f"head {head!r} needs at least 2 classes")
        c = cv_report(names, y, groups, cs, args.max_features)
        fitted[head] = fit_head(names, y, c, args.max_features)
        model["heads"][head] = export_head(*fitted[head])

    args.out.parent.mkdir(parents=True, exist_ok=True)
    args.out.write_text(json.dumps(model, separators=(",", ":")))
    print(f"\nwrote {args.out} ({args.out.stat().st_size / 1e6:.2f} MB)")

    cases, drift = [], 0.0
    for name in parity_names(rows):
        case: dict[str, Any] = {"name": name}
        for head, (vec, clf) in fitted.items():
            case[head] = predict_exported(model["heads"][head], vec, name)
            exact = clf.predict_proba(vec.transform([name]))[0]
            drift = max(
                drift, float(np.abs(np.array(list(case[head].values())) - exact).max())
            )
        cases.append(case)
    parity = args.parity or args.out.parent / "testdata" / "parity.json"
    parity.parent.mkdir(parents=True, exist_ok=True)
    parity.write_text(json.dumps(cases, indent=1, ensure_ascii=False))
    print(
        f"wrote {parity} ({len(cases)} cases; max drift from pruning/rounding {drift:.2e})"
    )


if __name__ == "__main__":
    main()
