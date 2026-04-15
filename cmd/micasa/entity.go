// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/micasa-dev/micasa/internal/data"
	"github.com/spf13/cobra"
	"gorm.io/gorm"
)

// entityDef defines the CRUD operations for a single entity type.
type entityDef[T any] struct {
	name        string // CLI name: "vendor", "service-log"
	singular    string // display: "vendor", "service log entry"
	tableHeader string // table header: "VENDORS", "SERVICE LOG"

	cols  []showCol[T]
	toMap func(T) map[string]any

	list func(*data.Store, bool) ([]T, error)
	get  func(*data.Store, string) (T, error)

	decodeAndCreate func(*data.Store, json.RawMessage) (T, error)
	decodeAndUpdate func(*data.Store, string, json.RawMessage) (T, error)

	del     func(*data.Store, string) error
	restore func(*data.Store, string) error

	deletedAt func(T) gorm.DeletedAt
}

// readInputData reads JSON from --data or --data-file flags. Exactly one
// must be provided.
func readInputData(cmd *cobra.Command) (json.RawMessage, error) {
	dataStr, _ := cmd.Flags().GetString("data")
	dataFile, _ := cmd.Flags().GetString("data-file")

	if dataStr != "" && dataFile != "" {
		return nil, errors.New("--data and --data-file are mutually exclusive")
	}
	if dataStr == "" && dataFile == "" {
		return nil, errors.New("--data or --data-file is required")
	}

	if dataFile != "" {
		b, err := os.ReadFile(dataFile) //nolint:gosec // user-specified input file
		if err != nil {
			return nil, fmt.Errorf("read data file: %w", err)
		}
		return json.RawMessage(b), nil
	}
	return json.RawMessage(dataStr), nil
}

// encodeJSON writes a value as indented JSON to w.
func encodeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	return nil
}

// mergeField unmarshals a field from a raw JSON map into dst if present.
func mergeField(fields map[string]json.RawMessage, key string, dst any) error {
	raw, ok := fields[key]
	if !ok {
		return nil
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("field %s: %w", key, err)
	}
	return nil
}

// parseFields unmarshals raw JSON into a field map for partial merging.
func parseFields(raw json.RawMessage) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	return fields, nil
}

// buildEntityCmd constructs the cobra command tree for an entity.
func buildEntityCmd[T any](def entityDef[T]) *cobra.Command {
	cmd := &cobra.Command{
		Use:           def.name,
		Short:         fmt.Sprintf("Manage %ss", def.singular),
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	if def.list != nil {
		cmd.AddCommand(buildListCmd(def))
	}
	if def.get != nil {
		cmd.AddCommand(buildGetCmd(def))
	}
	if def.decodeAndCreate != nil {
		cmd.AddCommand(buildAddCmd(def))
	}
	if def.decodeAndUpdate != nil {
		cmd.AddCommand(buildEditCmd(def))
	}
	if def.del != nil {
		cmd.AddCommand(buildDeleteCmd(def))
	}
	if def.restore != nil {
		cmd.AddCommand(buildRestoreCmd(def))
	}

	return cmd
}

func buildListCmd[T any](def entityDef[T]) *cobra.Command {
	var tableFlag bool
	var deletedFlag bool

	cmd := &cobra.Command{
		Use:           "list [database-path]",
		Short:         fmt.Sprintf("List %ss", def.singular),
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openExisting(dbPathFromEnvOrArg(args))
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()

			items, err := def.list(store, deletedFlag)
			if err != nil {
				return fmt.Errorf("list %ss: %w", def.singular, err)
			}

			cols, toMap := def.cols, def.toMap
			if deletedFlag && def.deletedAt != nil {
				cols, toMap = withDeletedCol(cols, toMap, true, def.deletedAt)
			}

			if tableFlag {
				return writeTable(cmd.OutOrStdout(), def.tableHeader, items, cols)
			}
			return writeJSON(cmd.OutOrStdout(), items, toMap)
		},
	}

	cmd.Flags().BoolVar(&tableFlag, "table", false, "Output as table")
	cmd.Flags().BoolVar(&deletedFlag, "deleted", false, "Include soft-deleted rows")
	return cmd
}

func buildGetCmd[T any](def entityDef[T]) *cobra.Command {
	var tableFlag bool

	cmd := &cobra.Command{
		Use:           "get <id> [database-path]",
		Short:         fmt.Sprintf("Get a %s by ID", def.singular),
		Args:          cobra.RangeArgs(1, 2),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			var dbPath string
			if len(args) > 1 {
				dbPath = args[1]
			}
			store, err := openExisting(dbPathFromEnvOrArgStr(dbPath))
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()

			item, err := def.get(store, args[0])
			if err != nil {
				return fmt.Errorf("get %s: %w", def.singular, err)
			}

			if tableFlag {
				return writeTable(cmd.OutOrStdout(), def.tableHeader, []T{item}, def.cols)
			}
			return encodeJSON(cmd.OutOrStdout(), def.toMap(item))
		},
	}

	cmd.Flags().BoolVar(&tableFlag, "table", false, "Output as table")
	return cmd
}

func buildAddCmd[T any](def entityDef[T]) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "add [database-path]",
		Short:         "Add a " + def.singular,
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openExisting(dbPathFromEnvOrArg(args))
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()

			raw, err := readInputData(cmd)
			if err != nil {
				return err
			}

			item, err := def.decodeAndCreate(store, raw)
			if err != nil {
				return err
			}
			return encodeJSON(cmd.OutOrStdout(), def.toMap(item))
		},
	}

	cmd.Flags().String("data", "", "JSON object with field values")
	cmd.Flags().String("data-file", "", "Path to JSON file with field values")
	return cmd
}

func buildEditCmd[T any](def entityDef[T]) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "edit <id> [database-path]",
		Short:         "Edit a " + def.singular,
		Args:          cobra.RangeArgs(1, 2),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			var dbPath string
			if len(args) > 1 {
				dbPath = args[1]
			}
			store, err := openExisting(dbPathFromEnvOrArgStr(dbPath))
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()

			raw, err := readInputData(cmd)
			if err != nil {
				return err
			}

			item, err := def.decodeAndUpdate(store, args[0], raw)
			if err != nil {
				return err
			}
			return encodeJSON(cmd.OutOrStdout(), def.toMap(item))
		},
	}

	cmd.Flags().String("data", "", "JSON object with fields to update")
	cmd.Flags().String("data-file", "", "Path to JSON file with fields to update")
	return cmd
}

func buildDeleteCmd[T any](def entityDef[T]) *cobra.Command {
	return &cobra.Command{
		Use:           "delete <id> [database-path]",
		Short:         "Delete a " + def.singular,
		Args:          cobra.RangeArgs(1, 2),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(_ *cobra.Command, args []string) error {
			var dbPath string
			if len(args) > 1 {
				dbPath = args[1]
			}
			store, err := openExisting(dbPathFromEnvOrArgStr(dbPath))
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()

			return def.del(store, args[0])
		},
	}
}

func buildRestoreCmd[T any](def entityDef[T]) *cobra.Command {
	return &cobra.Command{
		Use:           "restore <id> [database-path]",
		Short:         "Restore a deleted " + def.singular,
		Args:          cobra.RangeArgs(1, 2),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(_ *cobra.Command, args []string) error {
			var dbPath string
			if len(args) > 1 {
				dbPath = args[1]
			}
			store, err := openExisting(dbPathFromEnvOrArgStr(dbPath))
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()

			return def.restore(store, args[0])
		},
	}
}

// stringField extracts a string value from a raw JSON field map.
func stringField(fields map[string]json.RawMessage, key string) (string, bool) {
	raw, ok := fields[key]
	if !ok {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}

// dbPathFromEnvOrArgStr is like dbPathFromEnvOrArg but takes a string.
func dbPathFromEnvOrArgStr(s string) string {
	if s != "" {
		return s
	}
	return os.Getenv("MICASA_DB_PATH")
}

// newDBCmd creates the `db` parent command grouping all entity CRUD operations.
func newDBCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Read and write entity data",
	}
	cmd.AddCommand(
		newApplianceCmd(),
		newChatCmd(),
		newDeletionCmd(),
		newDocumentCmd(),
		newHouseCmd(),
		newIncidentCmd(),
		newMaintenanceCategoryCmd(),
		newMaintenanceCmd(),
		newProjectCmd(),
		newProjectTypeCmd(),
		newQuoteCmd(),
		newServiceLogCmd(),
		newVendorCmd(),
	)
	return cmd
}
