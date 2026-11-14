package engine

import (
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if err := EnableOTTLLambdaGate(); err != nil {
		fmt.Fprintf(os.Stderr, "enable OTTL lambda gate: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}
