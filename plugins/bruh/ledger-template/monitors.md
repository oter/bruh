# Monitors

The active monitors of the machine of bigm (spec 9.5). The live source is the MCP tool `monitor_list`. At each sweep, bigm rewrites the rows from `monitor_list` with `ledger_edit`: it adds a row for each new monitor, updates a changed row, and deletes the row of a monitor that ended in a commit `close waiting: <monitor ID> <source key>`. So this file can be one sweep out of date. A role reads `monitor_list`, never this file. The file holds the subscriptions only, never a polled value or an event. The column "Until" is the `until` of `monitor_list` as it is, or `standing` for a standing monitor of `repos_set`.

| ID | Subscriber | Project | Source key | Reason | Until |
|---|---|---|---|---|---|
