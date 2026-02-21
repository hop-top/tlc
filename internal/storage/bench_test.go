package storage

import (
	"context"
	"fmt"
	"os"
	"testing"

	"hop.top/tlc/internal/core"
)

func BenchmarkSQLiteStorage_ListTasks(b *testing.B) {
	dbPath := "test_bench.db"
	defer os.Remove(dbPath)

	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		b.Fatalf("failed to create storage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Seed with 1000 tasks
	for i := 0; i < 1000; i++ {
		task := &core.Task{
			ID:        fmt.Sprintf("T-%04d", i),
			Title:     fmt.Sprintf("Task %d", i),
			Status:    core.StatusTodo,
			Reference: "ref",
		}
		s.CreateTask(ctx, task)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q := core.Query{
			Search: "Task 500",
			Limit:  10,
		}
		_, err := s.ListTasks(ctx, q)
		if err != nil {
			b.Fatal(err)
		}
	}
}