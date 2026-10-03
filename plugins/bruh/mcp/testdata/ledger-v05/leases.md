# Leases

The lease table of the clankers. The bruh MCP server keeps the live table (`lease_list`). bigm copies it here at each sweep, with the time of the read. A clanker keeps the sub-leases of its clerks, and the project file shows them.

| Resource | Capacity | Holder (role key) | Until (UTC) | Source read |
|---|---|---|---|---|
| shop-db | 1 | clanker-shop | 2026-10-01T02:00:00Z | lease_list 2026-10-01T00:00:00Z |
