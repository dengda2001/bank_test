package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
)

// readDemoResults reads the newline-delimited or concatenated JSON snapshots
// used by the bank demo. It is kept outside the retired landlord importer so
// the unrelated demo fixture reader remains available.
func readDemoResults(path string) ([]demoResult, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	results := make([]demoResult, 0)
	for {
		var result demoResult
		if err := decoder.Decode(&result); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}
