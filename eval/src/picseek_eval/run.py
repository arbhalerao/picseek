import argparse
import os
import shutil
import sqlite3
import time
from collections import Counter
from pathlib import Path

from picseek_eval import corpus, stack
from picseek_eval.metrics import Scores, apply_threshold, summarize

CATEGORIES = ["keyword", "partial", "misread", "meaning", "visual"]
LAYER_SETS = [
    ("exact", ["exact"]),
    ("+ near", ["exact", "near"]),
    ("+ meaning", ["exact", "near", "meaning"]),
    ("+ visual (all)", ["exact", "near", "meaning", "visual"]),
]
PERMISSIVE = {
    "semantic_min": 0.0,
    "semantic_margin": 1.0,
    "visual_min": -1.0,
    "visual_margin": 1.0,
    "type_min": 0.0,
}
SHIPPED = {
    "semantic_min": 0.45,
    "semantic_margin": 0.15,
    "visual_min": 0.025,
    "visual_margin": 0.02,
    "type_min": 0.3,
}
TYPE_GRID = [round(0.20 + 0.05 * i, 2) for i in range(14)]
SEMANTIC_GRID = ([round(0.20 + 0.05 * i, 2) for i in range(10)], [0.05, 0.10, 0.15, 0.20, 1.0])
VISUAL_GRID = ([round(0.005 * i, 3) for i in range(13)], [0.01, 0.02, 0.03, 0.05, 1.0])
LABELS = ["money", "travel", "message", "dev", "security", "document", "chart"]


def calibrate(raw, queries, positives: set[str], grid, minimum_key: str, margin_key: str):
    pos = [q for q in queries if q.category in positives]
    neg = [q for q in queries if q.category == "negative"]
    best = None
    for minimum in grid[0]:
        for margin in grid[1]:
            recall = summarize(
                (apply_threshold(raw[q.text], minimum, margin)[:10], q.relevant) for q in pos
            ).recall10
            false_hits = sum(len(apply_threshold(raw[q.text], minimum, margin)) for q in neg)
            key = (false_hits == 0, recall, minimum, -margin)
            if best is None or key > best[0]:
                best = (key, {minimum_key: minimum, margin_key: margin}, recall, false_hits)
    return best[1], best[2], best[3]


def stored_types(db: Path) -> dict[str, tuple[str | None, float]]:
    conn = sqlite3.connect(f"file:{db}?mode=ro", uri=True)
    rows = conn.execute(
        "SELECT p.path, i.type, COALESCE(i.type_score, 0) "
        "FROM paths p JOIN images i ON i.id = p.image_id"
    )
    return {Path(path).name: (kind, score) for path, kind, score in rows}


def calibrate_types(shots, types) -> tuple[float, float]:
    truth = {s.name: s.kind for s in shots}
    best = (-1.0, 0.0)
    for threshold in TYPE_GRID:
        predicted = {
            n: (types[n][0] if types.get(n, (None, 0))[1] >= threshold else None) for n in truth
        }
        accuracy = sum(predicted[n] == truth[n] for n in truth) / len(truth)
        if accuracy > best[0]:
            best = (accuracy, threshold)
    return best[1], best[0]


def evaluate(stub, queries):
    table: dict[str, dict[str, Scores]] = {}
    for label, layers in LAYER_SETS:
        ranked = {
            q.text: [n for n, _ in stack.search(stub, q.text, layers, limit=20)] for q in queries
        }
        row = {
            c: summarize((ranked[q.text], q.relevant) for q in queries if q.category == c)
            for c in CATEGORIES
        }
        row["all"] = summarize(
            (ranked[q.text], q.relevant) for q in queries if q.category in CATEGORIES
        )
        row["negatives"] = sum(len(ranked[q.text]) for q in queries if q.category == "negative")
        table[label] = row
    return table


def classification(stub, shots):
    truth = {s.name: s.kind for s in shots}
    predicted = {name: None for name in truth}
    for label in LABELS:
        for name, _ in stack.search(stub, f"type:{label}", ["exact"], limit=200):
            if name in predicted:
                predicted[name] = label
    correct = sum(predicted[n] == truth[n] for n in truth)
    per_label = {}
    for label in [*LABELS, None]:
        tp = sum(predicted[n] == label and truth[n] == label for n in truth)
        fp = sum(predicted[n] == label and truth[n] != label for n in truth)
        fn = sum(predicted[n] != label and truth[n] == label for n in truth)
        if tp + fp + fn:
            per_label[label or "(none)"] = (tp, fp, fn)
    confusions = Counter(
        (truth[n] or "(none)", predicted[n] or "(none)") for n in truth if predicted[n] != truth[n]
    )
    return correct / len(truth), per_label, confusions


def report(shots, queries, thresholds, calibration, table, cls, timings) -> str:
    accuracy, per_label, confusions = cls
    lines = [
        "# picseek search quality (synthetic corpus)",
        "",
        f"{len(shots)} generated screenshots, "
        f"{sum(q.category in CATEGORIES for q in queries)} labelled queries "
        f"({', '.join(f'{c}: {sum(q.category == c for q in queries)}' for c in CATEGORIES)}), "
        f"{sum(q.category == 'negative' for q in queries)} negative queries. "
        "Corpus and queries: `eval/src/picseek_eval/corpus.py`.",
        "",
        "## Recall@10 / MRR by layer",
        "",
        "| Layers | " + " | ".join([*CATEGORIES, "all"]) + " | negatives returning results |",
        "|---|" + "---|" * (len(CATEGORIES) + 2),
    ]
    for label, row in table.items():
        cells = [f"{row[c].recall10:.2f} / {row[c].mrr:.2f}" for c in [*CATEGORIES, "all"]]
        lines.append(f"| {label} | " + " | ".join(cells) + f" | {row['negatives']} hits |")
    lines += [
        "",
        "## Recall@5 (all layers)",
        "",
        "| " + " | ".join([*CATEGORIES, "all"]) + " |",
        "|" + "---|" * (len(CATEGORIES) + 1),
        "| "
        + " | ".join(f"{table[LAYER_SETS[-1][0]][c].recall5:.2f}" for c in [*CATEGORIES, "all"])
        + " |",
        "",
        "## Thresholds",
        "",
        "All results above use the shipped defaults "
        f"(meaning >= {SHIPPED['semantic_min']} "
        f"within {SHIPPED['semantic_margin']} of the best hit; "
        f"visual lift >= {SHIPPED['visual_min']} within {SHIPPED['visual_margin']}; "
        f"type probability >= {SHIPPED['type_min']}).",
        "",
        "For reference, a grid search on this corpus (maximise recall@10 of the layer's own "
        "queries with every negative query returning nothing) picks looser values. They are not "
        "shipped: generated screenshots are cleaner than real collections, where off-topic "
        "queries score noticeably higher, so these values would admit noise. The shipped values "
        "sit at the knee of the recall/noise trade-off.",
        "",
        "| Layer (corpus-calibrated) | min | margin | recall@10 (layer alone) | negative hits |",
        "|---|---|---|---|---|",
        threshold_row("meaning", thresholds, "semantic", calibration["meaning"]),
        threshold_row("visual (lift)", thresholds, "visual", calibration["visual"]),
        "",
        f"Type filter: best accuracy on the corpus at probability >= {thresholds['type_min']}.",
        "",
        "## Type classification",
        "",
        f"Accuracy over all {len(shots)} images "
        f"(unlabelled ones should get no type): **{accuracy:.2f}**",
        "",
        "| Type | correct | wrongly assigned | missed |",
        "|---|---|---|---|",
    ]
    lines += [f"| {label} | {tp} | {fp} | {fn} |" for label, (tp, fp, fn) in per_label.items()]
    if confusions:
        lines += [
            "",
            "Most common confusions (true → predicted): "
            + ", ".join(f"{t} → {p} (x{n})" for (t, p), n in confusions.most_common(5)),
        ]
    lines += ["", "## Timings", "", *[f"- {k}: {v:.0f} s" for k, v in timings.items()], ""]
    return "\n".join(lines)


def threshold_row(label: str, thresholds, prefix: str, calibration) -> str:
    recall, false_hits = calibration
    minimum, margin = thresholds[f"{prefix}_min"], thresholds[f"{prefix}_margin"]
    return f"| {label} | {minimum} | {margin} | {recall:.2f} | {false_hits} |"


def main() -> None:
    parser = argparse.ArgumentParser(prog="picseek-eval")
    parser.add_argument("--work", type=Path, default=stack.REPO / "eval" / ".work")
    parser.add_argument(
        "--out", type=Path, default=stack.REPO / "eval" / "results" / "synthetic.md"
    )
    args = parser.parse_args()

    if not (stack.REPO / "bin" / "seekd").exists():
        raise SystemExit("bin/seekd not found; run `make build` first")
    runtime = Path(os.environ.get("XDG_RUNTIME_DIR") or "/tmp")
    socket_dir = runtime / "picseek-eval"
    shutil.rmtree(args.work, ignore_errors=True)
    corpus_dir, data_dir, config = (
        args.work / "corpus",
        args.work / "data",
        args.work / "config.toml",
    )
    log = args.work / "stack.log"
    timings: dict[str, float] = {}

    started = time.monotonic()
    shots = corpus.generate(corpus_dir)
    queries = corpus.queries()
    timings["generate corpus"] = time.monotonic() - started
    print(f"generated {len(shots)} screenshots and {len(queries)} queries in {args.work}")

    stack.write_config(config, corpus_dir, socket_dir, PERMISSIVE)
    started = time.monotonic()
    with stack.running(config, data_dir, socket_dir, log) as running:
        stub = running.stub
        state = stack.wait_until_indexed(running, len(shots))
        timings["index + enrich (cold start, incl. model loading)"] = time.monotonic() - started
        print(f"indexed: {state}")
        raw_meaning = {q.text: stack.search(stub, q.text, ["meaning"]) for q in queries}
        raw_visual = {q.text: stack.search(stub, q.text, ["visual"]) for q in queries}
        types = stored_types(data_dir / "picseek" / "index.db")

    meaning, meaning_recall, meaning_false = calibrate(
        raw_meaning, queries, {"meaning"}, SEMANTIC_GRID, "semantic_min", "semantic_margin"
    )
    visual, visual_recall, visual_false = calibrate(
        raw_visual, queries, {"visual"}, VISUAL_GRID, "visual_min", "visual_margin"
    )
    type_min, _ = calibrate_types(shots, types)
    thresholds = {**meaning, **visual, "type_min": type_min}
    print(f"calibrated thresholds: {thresholds}")

    stack.write_config(config, corpus_dir, socket_dir, SHIPPED)
    started = time.monotonic()
    with stack.running(config, data_dir, socket_dir, log) as running:
        stub = running.stub
        stack.wait_until_indexed(running, len(shots))
        timings["restart with calibrated thresholds"] = time.monotonic() - started
        table = evaluate(stub, queries)
        cls = classification(stub, shots)

    calibration = {
        "meaning": (meaning_recall, meaning_false),
        "visual": (visual_recall, visual_false),
    }
    text = report(shots, queries, thresholds, calibration, table, cls, timings)
    args.out.parent.mkdir(parents=True, exist_ok=True)
    args.out.write_text(text)
    print(text)
    print(f"written to {args.out}")


if __name__ == "__main__":
    main()
