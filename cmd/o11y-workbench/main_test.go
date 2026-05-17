package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func resetCfg() {
	cfgFile = ""
	viper.Reset()
}

func TestConfigFile(t *testing.T) {
	resetCfg()
	t.Cleanup(resetCfg)

	content := []byte("seed: 99\ntraces: 5\n")
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	cfgFile = cfgPath
	if err := initConfig(); err != nil {
		t.Fatal(err)
	}

	if got := viper.GetInt64("seed"); got != 99 {
		t.Errorf("seed = %d, want 99", got)
	}
	if got := viper.GetInt("traces"); got != 5 {
		t.Errorf("traces = %d, want 5", got)
	}
}

func TestConfigDefaults(t *testing.T) {
	resetCfg()
	t.Cleanup(resetCfg)

	var capturedSeed int64
	cmd := newRootCmd()
	cmd.SetArgs([]string{"--traces", "1"})
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		capturedSeed = viper.GetInt64("seed")
		return nil
	}

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if capturedSeed != 42 {
		t.Errorf("expected default seed 42, got %d", capturedSeed)
	}
}

func TestConfigCLIOverride(t *testing.T) {
	resetCfg()
	t.Cleanup(resetCfg)

	content := []byte("seed: 1\ntraces: 3\n")
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	cfgFile = cfgPath
	cmd := newRootCmd()
	cmd.SetArgs([]string{"--seed", "99", "--config", cfgPath})

	// Hook into RunE so we can verify viper state without actually running.
	var capturedSeed int64
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		capturedSeed = viper.GetInt64("seed")
		return nil
	}

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if capturedSeed != 99 {
		t.Errorf("expected CLI seed 99 to override config file, got %d", capturedSeed)
	}
}

func TestConfigEnvVar(t *testing.T) {
	resetCfg()
	t.Cleanup(resetCfg)

	const envKey = "O11Y_WORKBENCH_SEED"
	old := os.Getenv(envKey)
	os.Setenv(envKey, "77")
	t.Cleanup(func() { os.Setenv(envKey, old) })

	if err := initConfig(); err != nil {
		t.Fatal(err)
	}

	if got := viper.GetInt64("seed"); got != 77 {
		t.Errorf("expected env var seed 77, got %d", got)
	}
}

func TestConfigEnvVarOverridesConfigFile(t *testing.T) {
	resetCfg()
	t.Cleanup(resetCfg)

	const envKey = "O11Y_WORKBENCH_TRACES"
	old := os.Getenv(envKey)
	os.Setenv(envKey, "100")
	t.Cleanup(func() { os.Setenv(envKey, old) })

	content := []byte("traces: 5\n")
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	cfgFile = cfgPath
	if err := initConfig(); err != nil {
		t.Fatal(err)
	}

	if got := viper.GetInt("traces"); got != 100 {
		t.Errorf("expected env var 100 to override config file 5, got %d", got)
	}
}

func TestNewRootCmd_HasConfigFlag(t *testing.T) {
	cmd := newRootCmd()
	flag := cmd.PersistentFlags().Lookup("config")
	if flag == nil {
		t.Fatal("expected --config flag to exist")
	}
	if flag.DefValue != "" {
		t.Errorf("expected --config default to be empty, got %q", flag.DefValue)
	}
}

func TestNewRootCmd_HasScenarioFlag(t *testing.T) {
	cmd := newRootCmd()
	flag := cmd.Flags().Lookup("scenario")
	if flag == nil {
		t.Fatal("expected --scenario flag to exist")
	}
	if flag.DefValue != "" {
		t.Errorf("expected --scenario default to be empty, got %q", flag.DefValue)
	}
}

func TestNewRootCmd_HasScenarioSpanCountFlag(t *testing.T) {
	cmd := newRootCmd()
	flag := cmd.Flags().Lookup("scenario-span-count")
	if flag == nil {
		t.Fatal("expected --scenario-span-count flag to exist")
	}
	if flag.DefValue != "0" {
		t.Errorf("expected --scenario-span-count default to be 0, got %q", flag.DefValue)
	}
}
