package main

import (
	"fmt"
	"strings"
)

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// diffOp is one line of a diff: ' ', '-', or '+'.
type diffOp struct {
	kind   byte
	text   string
	ai, bi int // lines of a and b before this op
}

// maxLCSLines caps each side of the table of the longest common subsequence, so the
// table has at most about 4,000,000 cells. Above it, the diff shows the whole old file
// removed and the whole new file added.
const maxLCSLines = 2000

// unifiedDiff returns a unified diff with three lines of context, or "" when nothing changed.
// ponytail: O(n*m) longest common subsequence, capped at maxLCSLines on each side;
// fine for settings and template files, a whole-file replace for anything larger.
func unifiedDiff(from, to, old, new string) string {
	if old == new {
		return ""
	}
	a, b := splitLines(old), splitLines(new)
	n, m := len(a), len(b)
	var ops []diffOp
	if n > maxLCSLines || m > maxLCSLines {
		for i, line := range a {
			ops = append(ops, diffOp{'-', line, i, 0})
		}
		for j, line := range b {
			ops = append(ops, diffOp{'+', line, n, j})
		}
		return formatHunks(from, to, ops)
	}
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	for i, j := 0, 0; i < n || j < m; {
		switch {
		case i < n && j < m && a[i] == b[j]:
			ops = append(ops, diffOp{' ', a[i], i, j})
			i, j = i+1, j+1
		case i < n && (j == m || lcs[i+1][j] >= lcs[i][j+1]):
			ops = append(ops, diffOp{'-', a[i], i, j})
			i++
		default:
			ops = append(ops, diffOp{'+', b[j], i, j})
			j++
		}
	}
	return formatHunks(from, to, ops)
}

// formatHunks writes the ops as unified diff hunks with three lines of context.
func formatHunks(from, to string, ops []diffOp) string {
	const ctx = 3
	var out strings.Builder
	fmt.Fprintf(&out, "--- %s\n+++ %s\n", from, to)
	for k := 0; k < len(ops); {
		if ops[k].kind == ' ' {
			k++
			continue
		}
		start, end := max(0, k-ctx), k
		for end < len(ops) {
			if ops[end].kind != ' ' {
				end++
				continue
			}
			next := end
			for next < len(ops) && ops[next].kind == ' ' {
				next++
			}
			if next == len(ops) || next-end > 2*ctx {
				end = min(len(ops), end+ctx)
				break
			}
			end = next
		}
		oc, nc := 0, 0
		for _, o := range ops[start:end] {
			if o.kind != '+' {
				oc++
			}
			if o.kind != '-' {
				nc++
			}
		}
		os, ns := ops[start].ai+1, ops[start].bi+1
		if oc == 0 {
			os--
		}
		if nc == 0 {
			ns--
		}
		fmt.Fprintf(&out, "@@ -%d,%d +%d,%d @@\n", os, oc, ns, nc)
		for _, o := range ops[start:end] {
			out.WriteByte(o.kind)
			out.WriteString(o.text)
			out.WriteByte('\n')
		}
		k = end
	}
	return out.String()
}
