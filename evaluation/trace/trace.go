// Package trace reads the durable JSONL trace a BuildMax run records.
//
// It exists because three places in evaluation need to ask a trace a question —
// the CLI adapter counts model calls, the trace grader asserts on tool use, and
// the Harbor importer counts model calls for an external run — and the bounds
// and failure rules for reading one are the same every time. Only the questions
// differ.
//
// Records are decoded field by field rather than through the runtime's own
// record type. A bundle outlives the build that wrote it, and section 8.4
// requires the format to stay readable without a BuildMax process; decoding
// through a struct that has since gained or lost fields would make an old trace
// unreadable rather than merely incomplete.
package trace

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

// MaxLine bounds one trace record. The recorder bounds each free-text field at
// 4 KiB, so a record far above this is corruption rather than a large tool
// result.
const MaxLine = 4 * 1024 * 1024

// Scanner reads a trace file line by line under the shared bounds.
func Scanner(f *os.File) *bufio.Scanner {
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 0, 64*1024), MaxLine)
	return s
}

// CountLLMCalls reports how many model calls a trace recorded.
//
// The print-mode envelope carries tool calls and tokens but not this, and
// reliability reporting needs to tell one expensive call from ten cheap ones.
//
// A read that stops early returns its error rather than the count so far. A
// truncated count is indistinguishable from a cheap run, and quietly reporting
// one as the other is the kind of wrong number a qualification would act on.
func CountLLMCalls(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()

	count := 0
	scanner := Scanner(f)
	for scanner.Scan() {
		var rec struct {
			Type string `json:"type"`
		}
		// A line that does not parse is not this function's to reject: it
		// counts model calls, and trace validity is a grader's question.
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			continue
		}
		if rec.Type == "llm_end" {
			count++
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("read trace: %w", err)
	}
	return count, nil
}
