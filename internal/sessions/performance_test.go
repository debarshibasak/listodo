package sessions

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/adaptive-scale/listodo/internal/model"
)

func BenchmarkScan(b *testing.B) {
	dir := b.TempDir()
	now := time.Now()
	for i := range 40 {
		body := fmt.Sprintf("{\"type\":\"session_meta\",\"timestamp\":%q,\"payload\":{\"id\":%q,\"cwd\":\"/work\"}}\n", now.Format(time.RFC3339Nano), fmt.Sprint(i)) +
			"{\"type\":\"response_item\",\"payload\":{\"type\":\"message\",\"role\":\"user\",\"content\":\"Implement a task\"}}\n" +
			strings.Repeat("{\"type\":\"response_item\",\"payload\":{\"type\":\"function_call_output\",\"output\":\""+strings.Repeat("x", 2048)+"\"}}\n", 32)
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%d.jsonl", i)), []byte(body), 0600); err != nil {
			b.Fatal(err)
		}
	}
	settings := model.Settings{CodexEnabled: true, CodexRoot: dir}
	b.Run("full", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if report := Scan(settings, now); len(report.Sessions) != 40 {
				b.Fatal("missing sessions")
			}
		}
	})
	b.Run("unchanged", func(b *testing.B) {
		var scanner Scanner
		scanner.Scan(settings, now)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if report := scanner.Scan(settings, now); report.Count != 40 || len(report.Sessions) != 0 {
				b.Fatal("unchanged transcripts reparsed")
			}
		}
	})
}
