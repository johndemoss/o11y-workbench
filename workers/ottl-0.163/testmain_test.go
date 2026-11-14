package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/johndemoss/o11y-workbench/workers/ottl-0.163/engine"
)

func TestMain(m *testing.M) {
	if err := engine.EnableOTTLLambdaGate(); err != nil {
		fmt.Fprintf(os.Stderr, "enable OTTL lambda gate: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}
