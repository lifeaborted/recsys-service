package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"recsys/internal/contract"
	"recsys/internal/signals"
)

type fileWriter string

func (p fileWriter) PublishSnapshot(ctx context.Context, s contract.Snapshot) error {
	f, err := os.CreateTemp(filepath.Dir(string(p)), ".snapshot-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	err = json.NewEncoder(f).Encode(s)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return os.Rename(f.Name(), string(p))
}

func run() error {
	in := flag.String("input", "", "input JSON containing Snapshot metadata and Data (DatasetBatch)")
	out := flag.String("output", "", "destination snapshot JSON")
	database, snapshot, dataset, cutoff, evidence := registerDatabaseFlags()
	flag.Parse()
	if *database {
		if *in != "" || *out != "" {
			return fmt.Errorf("database mode does not take -input or -output")
		}
		return runDatabase(context.Background(), *snapshot, *dataset, *cutoff, *evidence)
	}
	if *in == "" || *out == "" {
		return fmt.Errorf("supply -input and -output")
	}
	f, err := os.Open(*in)
	if err != nil {
		return err
	}
	defer f.Close()
	var input struct {
		Snapshot contract.Snapshot
		Data     contract.DatasetBatch
	}
	if err = json.NewDecoder(f).Decode(&input); err != nil {
		return err
	}
	return signals.Build(context.Background(), input.Snapshot, input.Data, fileWriter(*out))
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
