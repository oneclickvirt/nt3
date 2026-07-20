package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/oneclickvirt/nt3/model"
)

const defaultSourceURL = "https://raw.githubusercontent.com/xykt/NetQuality/main/ref/province.json"

type updateConfig struct {
	Source   string
	Output   string
	Manifest string
	Timeout  time.Duration
}

func main() {
	config := updateConfig{}
	flag.StringVar(&config.Source, "source", defaultSourceURL, "upstream province metadata URL")
	flag.StringVar(&config.Output, "output", "model/snapshot/province-routes.json", "snapshot output path")
	flag.StringVar(&config.Manifest, "manifest", "model/snapshot/manifest.json", "snapshot manifest output path")
	flag.DurationVar(&config.Timeout, "timeout", 30*time.Second, "upstream request timeout")
	flag.Parse()
	if err := updateSnapshot(context.Background(), http.DefaultClient, config); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func updateSnapshot(ctx context.Context, client *http.Client, config updateConfig) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if client == nil {
		client = http.DefaultClient
	}
	if config.Source == "" || config.Output == "" || config.Timeout <= 0 {
		return errors.New("source, output, and timeout must be valid")
	}
	requestCtx, cancel := context.WithTimeout(ctx, config.Timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, config.Source, nil)
	if err != nil {
		return fmt.Errorf("create province request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "oneclickvirt-nt3-province-sync/1")
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("fetch province metadata: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch province metadata: HTTP %d", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return err
	}
	var entries []model.ProvinceRouteSnapshotEntry
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&entries); err != nil {
		return fmt.Errorf("decode province metadata: %w", err)
	}
	routes, err := model.BuildProvinceRoutesFromMetadata(entries)
	if err != nil {
		return fmt.Errorf("validate province metadata: %w", err)
	}
	encoded, err := json.Marshal(routes)
	if err != nil {
		return err
	}
	candidate, err := model.NormalizeProvinceRouteSnapshot(encoded)
	if err != nil {
		return err
	}
	return replaceSnapshot(config.Output, config.Manifest, candidate)
}

type snapshotManifest struct {
	Schema      string `json:"schema"`
	File        string `json:"file"`
	Count       int    `json:"count"`
	SHA256      string `json:"sha256"`
	GeneratedAt string `json:"generated_at"`
}

func replaceSnapshot(output, manifestOutput string, candidate []byte) error {
	routes, err := model.ParseProvinceRoutes(candidate)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(candidate)
	manifest := snapshotManifest{Schema: model.ProvinceRouteRegistrySchema, File: filepath.Base(output), Count: len(routes), SHA256: hex.EncodeToString(hash[:]), GeneratedAt: time.Now().UTC().Format(time.RFC3339)}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	manifestData = append(manifestData, '\n')
	current, readErr := os.ReadFile(output)
	if readErr == nil {
		if normalized, normalizeErr := model.NormalizeProvinceRouteSnapshot(current); normalizeErr == nil && bytes.Equal(normalized, candidate) && manifestMatches(manifestOutput, candidate, len(routes)) {
			return nil
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return fmt.Errorf("read existing snapshot: %w", readErr)
	}
	if err := writeAtomicSnapshot(output, candidate); err != nil {
		return err
	}
	return writeAtomicSnapshot(manifestOutput, manifestData)
}

func manifestMatches(path string, snapshot []byte, count int) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var manifest snapshotManifest
	if json.Unmarshal(data, &manifest) != nil || manifest.Schema != model.ProvinceRouteRegistrySchema || manifest.File != "province-routes.json" || manifest.Count != count {
		return false
	}
	hash := sha256.Sum256(snapshot)
	return manifest.SHA256 == hex.EncodeToString(hash[:])
}

func writeAtomicSnapshot(output string, data []byte) error {
	if output == "" {
		return errors.New("snapshot path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(output), ".province-routes-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, output)
}
