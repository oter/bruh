# Where to get the review guides

Put a copy of each guide into `.scratch/<feature>/guides/`, one file for each guide, with the first line `Source: <url>, fetched <date>`. Get it with WebFetch (ask for the page as Markdown), or with `curl -L` on a raw GitHub URL. Then write `guides/INDEX.md`: a table from file globs to guide files, and the accepted deviations that reviewers must not flag. Reviewers apply only the MUST rules, and cite the rule ID or the heading for each finding.

Pick the guides for each technology of the diff. When a technology has no row here, add the row when you first use it.

## Go

| Guide | Source | Use for |
|---|---|---|
| JetBrains Modern Go Guidelines | <https://github.com/JetBrains/go-modern-guidelines> (the README and `guidelines/`) | idioms for each Go version: `min` and `max`, the `slices` and `maps` packages, range over an integer, `errors.Join`, `errors.AsType`, `encoding/json/v2`, `testing/synctest` |
| Google Go Style Guide | <https://google.github.io/styleguide/go/guide>, <https://google.github.io/styleguide/go/decisions>, <https://google.github.io/styleguide/go/best-practices> | names, doc comments, error strings, wrapping, receiver names, package layout |
| Effective Go and Go Code Review Comments | <https://go.dev/doc/effective_go>, <https://go.dev/wiki/CodeReviewComments> | the baseline that the two guides above assume |
| Go wiki TestComments | <https://go.dev/wiki/TestComments> | test names, failure messages, table tests, `t.Fatal` and `t.Error` |
| testcontainers-go docs | <https://golang.testcontainers.org/features/creating_container/>, <https://golang.testcontainers.org/features/garbage_collector/> | the container lifecycle in `TestMain`, cleanup, the reaper |
| golangci-lint linters | <https://golangci-lint.run/usage/linters/> | the static checks that the gate covers already, so reviewers skip them |

## TypeScript and React

| Guide | Source | Use for |
|---|---|---|
| Google TypeScript Style Guide | <https://google.github.io/styleguide/tsguide.html> | names, imports, types and interfaces, null handling, exports |
| TypeScript handbook, Do's and Don'ts | <https://www.typescriptlang.org/docs/handbook/declaration-files/do-s-and-don-ts.html> | anti-patterns at the type level |
| typescript-eslint rules | <https://typescript-eslint.io/rules/> | the rules that a linter enforces already (filter on recommended-type-checked) |
| Vercel agent skills | <https://github.com/vercel-labs/agent-skills> (the skills `react-best-practices` and `web-design-guidelines`) | server and client components, data fetching, waterfalls, bundle size; interaction states, focus, motion, layout |
| Next.js docs, App Router | <https://nextjs.org/docs/app> | routes, caching, the semantics of server actions |
| React docs, Rules of React | <https://react.dev/reference/rules> | purity, the rules of hooks, effects |

## HTML, CSS, and accessibility

| Guide | Source | Use for |
|---|---|---|
| Google HTML/CSS Style Guide | <https://google.github.io/styleguide/htmlcssguide.html> | format, semantics, class names, property order |
| web.dev Learn HTML and Learn CSS | <https://web.dev/learn/html>, <https://web.dev/learn/css> | semantics, the cascade, layout, logical properties |
| MDN HTML and CSS reference | <https://developer.mozilla.org/en-US/docs/Web/HTML>, <https://developer.mozilla.org/en-US/docs/Web/CSS> | the fact check for each element and property |
| WCAG 2.2 quick reference | <https://www.w3.org/WAI/WCAG22/quickref/> | contrast, keyboard, names and roles, focus order |
| WAI-ARIA Authoring Practices | <https://www.w3.org/WAI/ARIA/apg/patterns/> | the correct roles and keyboard behavior of each widget |

## API, infrastructure, CI, and data

| Guide | Source | Use for |
|---|---|---|
| Spectral OpenAPI rules | <https://docs.stoplight.io/docs/spectral/4dec24461f3af-open-api-rules> | operation IDs, descriptions, tags, unused components, declared security |
| OpenAPI 3.1 spec | <https://spec.openapis.org/oas/v3.1.0> | the fact check |
| Terraform style guide | <https://developer.hashicorp.com/terraform/language/style> | names, file layout, variables, versions |
| Docker build best practices | <https://docs.docker.com/build/building/best-practices/> | layers, multi-stage builds, pins, a user that is not root |
| GitLab CI/CD YAML reference | <https://docs.gitlab.com/ci/yaml/> | keywords, `rules`, `stages`, `image`, `workflow`, the default pipeline sources |
| GitLab Docker-in-Docker | <https://docs.gitlab.com/ci/docker/using_docker_build/> | the dind service, TLS, `DOCKER_HOST` |
| Testcontainers in GitLab CI | <https://golang.testcontainers.org/system_requirements/ci/gitlab_ci/> | the runner setup for containers in tests |
| GitHub Actions workflow syntax | <https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax> | keywords, permissions, triggers, job matrices |
| Firestore best practices | <https://firebase.google.com/docs/firestore/best-practices> | indexes, hot documents, transactions, ID design |
| Firestore TTL | <https://firebase.google.com/docs/firestore/ttl> | one expiry field for each collection, a timestamp type, the deletion lag |

## Python and shell

| Guide | Source | Use for |
|---|---|---|
| Google Python Style Guide | <https://google.github.io/styleguide/pyguide.html> | names, imports, docstrings, type annotations, comprehensions, exceptions |
| PEP 8 | <https://peps.python.org/pep-0008/> | the baseline format and names that the Google guide assumes |
| Google Shell Style Guide | <https://google.github.io/styleguide/shellguide.html> | when to use shell, quotes, functions, error handling |

## Operations

| Guide | Source | Use for |
|---|---|---|
| Ansible tips and tricks | <https://docs.ansible.com/ansible/latest/tips_tricks/ansible_tips_tricks.html> | playbook structure, idempotence, check mode, secrets |
| Ansible blocks | <https://docs.ansible.com/ansible/latest/playbook_guide/playbooks_blocks.html> | `block`, `rescue`, and `always` for error handling and rollback |
| ansible-lint rules | <https://ansible.readthedocs.io/projects/lint/rules/> | `no-changed-when`, `risky-shell-pipe`, `command-instead-of-shell`, `no-log-password`, `fqcn` |
| PostgreSQL pg_dump and pg_restore | <https://www.postgresql.org/docs/current/app-pgdump.html>, <https://www.postgresql.org/docs/current/app-pgrestore.html> | the dump direction across versions, `-Fc`, `--no-owner`, `--exit-on-error` |

## Security

| Guide | Source | Use for |
|---|---|---|
| OWASP Cheat Sheets: Authentication, Session Management, Password Storage, Forgot Password | <https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html> and the pages next to it | a generic 401, timing, lockout, hashes, reset flows |
| OWASP ASVS 5.0 | <https://github.com/OWASP/ASVS/tree/master/5.0/en> | numbered requirements to cite for each finding |
| OWASP Top 10 | <https://owasp.org/Top10/> | the lens list of a security reviewer |

## Design principles

| Guide | Source | Use for |
|---|---|---|
| Go Proverbs (Rob Pike) | <https://go-proverbs.github.io/> | Go idioms: small interfaces, "a little copying is better than a little dependency", "clear is better than clever" |
| Effective Go | <https://go.dev/doc/effective_go> | idiomatic Go design: interfaces, embedding, errors |
| DRY and orthogonality (Hunt and Thomas, The Pragmatic Programmer) | <https://www.artima.com/articles/orthogonality-and-the-dry-principle> | one authoritative representation of each piece of knowledge; no coupling of unrelated parts |
| SOLID (Robert C. Martin) | <http://butunclebob.com/ArticleS.UncleBob.PrinciplesOfOod> (HTTP only: get it with `curl -L`, because WebFetch upgrades to HTTPS and the host refuses it), <https://blog.cleancoder.com/uncle-bob/2014/05/08/SingleReponsibilityPrinciple.html>, <https://blog.cleancoder.com/uncle-bob/2020/10/18/Solid-Relevance.html> | SRP, OCP, LSP, ISP, and DIP |

## Decisions and docs

| Guide | Source | Use for |
|---|---|---|
| Nygard ADR template | <https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions> | the context, decision, and consequences shape of a decision entry |
| Google developer documentation style guide | <https://developers.google.com/style> | the words of README files and docs |
| ASD-STE100 Simplified Technical English | <https://www.asd-ste100.org/> | short sentences, one meaning for each word, active voice |
