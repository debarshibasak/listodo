package desktop

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/adaptive-scale/listodo/internal/model"
	"github.com/adaptive-scale/listodo/internal/store"
)

// Model a history with bounded logs: listing it should allocate for metadata,
// rather than retaining and decoding every run's output.
func BenchmarkOperationList(b *testing.B) {
	db, err := store.Open(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	task, err := db.SaveTask(model.Task{Title: "Benchmark", Status: "todo", Priority: 1})
	if err != nil {
		b.Fatal(err)
	}
	for i := range 100 {
		err := db.BeginOperation(model.Operation{
			ID: fmt.Sprint(i), TaskID: task.ID, Title: task.Title, Provider: "codex",
			Status: "succeeded", StartedAt: time.Now(), UpdatedAt: time.Now(),
			Output: strings.Repeat("x", operationOutputLimit), Prompt: strings.Repeat("p", 1000),
		})
		if err != nil {
			b.Fatal(err)
		}
	}
	server, err := NewServer(db)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		response := operationRequest(server, "GET", "/api/operations", nil)
		if response.Code != 200 {
			b.Fatal(response.Body.String())
		}
	}
}
