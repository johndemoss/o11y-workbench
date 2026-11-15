package ottlengine

import (
	"context"
	"fmt"
	"io"
	"os"
	"testing"
	"time"
)

func init() {
	if os.Getenv("O11Y_TEST_WORKER") == "1" {
		fmt.Println("READY http://127.0.0.1:12345")
		_, _ = io.Copy(io.Discard, os.Stdin)
		os.Exit(0)
	}
}

func TestWorkerStopReapsProcess(t *testing.T) {
	t.Setenv("O11Y_TEST_WORKER", "1")
	_, done, stop, err := StartWorker(context.Background(), os.Args[0], "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("worker was not reaped")
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
}
