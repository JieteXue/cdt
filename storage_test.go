package main

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestReaderSnapshotDuringReplace(t *testing.T) {
	file := seedRegistry(t)
	reader, err := openRegistry(file)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if err := update(file, func(r *Registry) error {
		r.Entries[0].Tags = []string{"updated"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(reader)
	if err != nil || len(data) == 0 {
		t.Fatal("reader lost its snapshot during replacement", err)
	}
	r, err := load(file)
	if err != nil || r.Entries[0].Tags[0] != "updated" {
		t.Fatal(r, err)
	}
}

func TestConcurrentWriterProcesses(t *testing.T) {
	file := filepath.Join(t.TempDir(), "cdt", "registry.json")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	const count = 8
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cmd := exec.CommandContext(ctx, exe, "-test.run=^TestRegistryWriterProcess$")
			cmd.Env = append(os.Environ(), "CDT_WRITER_FILE="+file, "CDT_WRITER_ID="+strconv.Itoa(i))
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("writer: %v: %s", err, out)
			}
		}(i)
	}
	wg.Wait()
	r, err := load(file)
	if err != nil || len(r.Entries) != count {
		t.Fatalf("lost concurrent writes: %+v, %v", r, err)
	}
}

func TestRegistryWriterProcess(t *testing.T) {
	file := os.Getenv("CDT_WRITER_FILE")
	if file == "" {
		return
	}
	if err := update(file, func(r *Registry) error {
		r.Entries = append(r.Entries, Entry{testPath("/writer-" + os.Getenv("CDT_WRITER_ID")), []string{"tag"}})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
