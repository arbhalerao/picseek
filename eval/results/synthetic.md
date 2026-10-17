# picseek search quality (synthetic corpus)

43 generated screenshots, 35 labelled queries (keyword: 9, partial: 3, misread: 5, meaning: 11, visual: 7), 4 negative queries. Corpus and queries: `eval/src/picseek_eval/corpus.py`.

## Recall@10 / MRR by layer

| Layers | keyword | partial | misread | meaning | visual | all | negatives returning results |
|---|---|---|---|---|---|---|---|
| exact | 1.00 / 1.00 | 1.00 / 1.00 | 0.00 / 0.00 | 0.00 / 0.00 | 0.00 / 0.00 | 0.34 / 0.34 | 0 hits |
| + near | 1.00 / 1.00 | 1.00 / 1.00 | 1.00 / 1.00 | 0.00 / 0.00 | 0.00 / 0.00 | 0.49 / 0.49 | 0 hits |
| + meaning | 1.00 / 1.00 | 1.00 / 1.00 | 1.00 / 1.00 | 0.85 / 0.91 | 0.00 / 0.00 | 0.75 / 0.77 | 0 hits |
| + visual (all) | 1.00 / 1.00 | 1.00 / 1.00 | 1.00 / 1.00 | 0.85 / 0.91 | 0.74 / 0.71 | 0.90 / 0.91 | 0 hits |

## Recall@5 (all layers)

| keyword | partial | misread | meaning | visual | all |
|---|---|---|---|---|---|
| 1.00 | 1.00 | 1.00 | 0.85 | 0.74 | 0.90 |

## Thresholds

All results above use the shipped defaults (meaning >= 0.3 within 0.2 of the best hit; visual lift >= 0.02 within 0.02; type probability >= 0.3).

For reference, a grid search on this corpus (maximise recall@10 of the layer's own queries with every negative query returning nothing) picks looser values. They are not shipped: generated screenshots are cleaner than real collections, where off-topic queries score noticeably higher, so these values would admit noise. The shipped values sit at the knee of the recall/noise trade-off.

| Layer (corpus-calibrated) | min | margin | recall@10 (layer alone) | negative hits |
|---|---|---|---|---|
| meaning | 0.2 | 1.0 | 0.91 | 0 |
| visual (lift) | 0.01 | 0.03 | 0.82 | 0 |

Type filter: best accuracy on the corpus at probability >= 0.35.

## Type classification

Accuracy over all 43 images (unlabelled ones should get no type): **0.95**

| Type | correct | wrongly assigned | missed |
|---|---|---|---|
| money | 7 | 1 | 0 |
| travel | 2 | 0 | 0 |
| message | 4 | 0 | 0 |
| dev | 6 | 0 | 0 |
| security | 3 | 0 | 0 |
| document | 11 | 1 | 0 |
| chart | 3 | 0 | 0 |
| (none) | 5 | 0 | 2 |

Most common confusions (true → predicted): (none) → money (x1), (none) → document (x1)

## Timings

- generate corpus: 1 s
- index + enrich (cold start, incl. model loading): 18 s
- restart with calibrated thresholds: 3 s
