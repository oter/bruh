# Mode

mode: human
changed: none
reason: default of the ledger template
p1_batch_minutes: 60
p1_batch_size: 5
review_round_cap: 2
status_cadence: on-change
ledger_max_lines: 300
remote_environments: none
monitor_default_hours: 24
monitor_max_hours: 168
monitor_max_active: 20

## Keys

- `mode`: `human` or `autonomous`. In human mode, P0 and P1 questions go to the owner. In autonomous mode, bigm decides P1 questions itself and records each decision with the tag "bigm decision <date>" and its reasons. The never-without-the-owner list of `priorities.md` is a hard stop in both modes.
- `changed`: the UTC time of the last change of `mode`, from `date -u`.
- `reason`: the reason of the last change of `mode`.
- `p1_batch_minutes`: the interval of the P1 batch, in minutes.
- `p1_batch_size`: the maximum number of items in a P1 batch. A batch goes out early when this number of items is queued.
- `review_round_cap`: the review-round cap of the `deliver` workflow.
- `status_cadence`: `on-change` (bigm reports status only when something changed) or `always` (bigm reports status at each sweep).
- `ledger_max_lines`: the line cap of each Markdown file of the ledger. At each sweep, bigm deletes the rows of closed items that it missed in a longer file. If the file stays longer, bigm sends one P1. The default is 300.
- `remote_environments`: the paired Orca environments that bigm can use, comma-separated, or `none`. bigm writes this line when the owner names a remote machine.
- `monitor_default_hours`: the lifetime of a monitor when `monitor_start` gets no `hours`. The default is 24.
- `monitor_max_hours`: the largest `hours` that `monitor_start` accepts. The default is 168.
- `monitor_max_active`: the most source keys that are active on the machine of bigm at the same time, standing monitors included. `monitor_start` refuses a new source key past it. The default is 20.

bigm reads this file at the start of each turn. The mode and the P1 settings change only through bigm, on an explicit answer of the owner.
