# Mode

mode: autonomous
changed: 2026-09-30T12:00:00Z
reason: owner answer Q-2
p1_batch_minutes: 45
p1_batch_size: 5
review_round_cap: 3
status_cadence: always

## Keys

- `mode`: `human` or `autonomous`. In human mode, P0 and P1 questions go to the owner. In autonomous mode, bigm decides P1 questions itself and records each decision with the tag "bigm decision <date>" and its reasons. The never-without-the-owner list of `priorities.md` is a hard stop in both modes.
- `changed`: the UTC time of the last change of `mode`, from `date -u`.
- `reason`: the reason of the last change of `mode`.
- `p1_batch_minutes`: the interval of the P1 batch, in minutes.
- `p1_batch_size`: the maximum number of items in a P1 batch. A batch goes out early when this number of items is queued.
- `review_round_cap`: the review-round cap of the `deliver` workflow.
- `status_cadence`: `on-change` (bigm reports status only when something changed) or `always` (bigm reports status at each sweep).

bigm reads this file at the start of each turn. Only the owner changes it, or bigm on an explicit owner answer.
