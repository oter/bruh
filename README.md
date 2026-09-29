# bruh

bruh is a Claude Code plugin that coordinates work across Claude Code sessions and machines. It contains a skillset and workflows.

## Roles

| Role | Scope | Does hands-on work |
|---|---|---|
| bigm | All projects. Starts clanker sessions in project folders. | No |
| Clanker | One project. Knows the full picture of that project. Gives tasks to clerks. | No |
| Clerk | One task. Starts workflows that do the work. | No |
| Workflow | One step of a task. | Yes |

## Status

The design is in progress. No decision is final. The plugin is not available yet.

- [Specification](docs/spec.md): the input for the implementation plan.
- [Flow](docs/flow.md): the process as diagrams: topology, task lifecycle, and the question, delivery, context, and liveness loops.
- [Design](docs/design.md): what the owner said, the decisions, and the open questions.
- [Knowledge](docs/knowledge.md): the verified facts about Claude Code and Orca that the design depends on.
