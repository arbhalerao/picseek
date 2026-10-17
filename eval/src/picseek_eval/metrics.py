from collections.abc import Iterable, Sequence
from dataclasses import dataclass


@dataclass(frozen=True)
class Scores:
    recall5: float
    recall10: float
    mrr: float
    count: int


def recall_at(ranked: Sequence[str], relevant: frozenset[str], k: int) -> float:
    return len(relevant.intersection(ranked[:k])) / len(relevant)


def reciprocal_rank(ranked: Sequence[str], relevant: frozenset[str]) -> float:
    for i, name in enumerate(ranked):
        if name in relevant:
            return 1 / (i + 1)
    return 0.0


def summarize(results: Iterable[tuple[Sequence[str], frozenset[str]]]) -> Scores:
    r5 = r10 = rr = 0.0
    n = 0
    for ranked, relevant in results:
        r5 += recall_at(ranked, relevant, 5)
        r10 += recall_at(ranked, relevant, 10)
        rr += reciprocal_rank(ranked, relevant)
        n += 1
    if n == 0:
        return Scores(0, 0, 0, 0)
    return Scores(r5 / n, r10 / n, rr / n, n)


def apply_threshold(hits: Sequence[tuple[str, float]], minimum: float, margin: float) -> list[str]:
    if not hits:
        return []
    top = max(score for _, score in hits)
    return [name for name, score in hits if score >= minimum and score >= top - margin]
